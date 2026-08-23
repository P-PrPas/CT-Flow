"""Long inference passes over a folder (score/evaluate/autolabel/reembed) all
run as a background job: the request returns a job_id immediately, and the UI
polls GET /api/jobs/{id} for progress. See services/job_tracker.py.

Every job-starting endpoint below returns the same shape ({job_id, total}) and
is polled the same way; only `result`'s shape once `finished` differs per job
-- documented on each endpoint since GET /api/jobs/{id} itself can't know
which kind of job it's looking at.

The inference itself happens in the sidecar (backend/vpe_service.py), which
streams one line per image back. This router owns the job tracker, the
database writes, and the metrics -- so the shape of every result below is
unchanged even though nothing here loads a model any more."""
import time

from fastapi import APIRouter, BackgroundTasks, HTTPException

from .. import deps
from ..deps import checked_path
from ..services import annotations_db, job_tracker, metrics, vpe_client
from ..services import models as model_registry
from .pool import bank_summary

router = APIRouter(prefix="/api", tags=["jobs"])


@router.get("/jobs/{job_id}")
def get_job(job_id: str):
    """Poll target for every background job below. `404` once a job_id was
    never issued -- jobs are never pruned otherwise (see job_tracker.py's
    ponytail note), so this only fires for a typo'd or pre-restart id."""
    j = job_tracker.get(job_id)
    if j is None:
        raise HTTPException(404, "unknown job")
    return j | {"now": time.time()}


def _bank_or_400(state_dir: str) -> dict:
    """Every job below is meaningless against an empty bank, and finding that
    out here costs one cheap call instead of a job that fails immediately."""
    b = vpe_client.bank(state_dir)
    if not b["classes"]:
        raise HTTPException(400, "prompt bank is empty -- label something first")
    return b


@router.post("/score")
def score(req: dict, background_tasks: BackgroundTasks):
    """Rescore the pool with the current bank. Runs as a background job so
    the UI can show progress -- a few hundred images takes real time.

    Job result: `{"scores": {image_path: {"conf": float, "cls": str|None, "sig": [int, ...]}}}`."""
    paths = [str(checked_path(p)) for p in req["images"]]
    state_dir = str(deps.state_dir(checked_path(req["input_dir"])))
    job_id = job_tracker.create(len(paths))
    if not vpe_client.bank(state_dir)["classes"] or not paths:
        job_tracker.finish(job_id, {"scores": {}})
        return {"job_id": job_id, "total": 0}
    background_tasks.add_task(_run_score, job_id, state_dir, paths)
    return {"job_id": job_id, "total": len(paths)}


def _run_score(job_id: str, state_dir: str, paths: list[str]):
    try:
        results = {}
        done = 0
        # conf=0.05 rather than the usual default: this pass exists to rank
        # images by "how sure is the model here", so a box below the labelling
        # threshold is still a signal worth recording.
        for line in vpe_client.predict_stream(state_dir, paths, conf=0.05,
                                              conf_by_class={}, want_sig=True):
            if line.get("done"):
                break
            best = max(line["boxes"], key=lambda d: d["conf"], default=None)
            results[line["image"]] = {
                "conf": best["conf"] if best else 0.0,
                "cls": best["cls"] if best else None,
                "sig": line["sig"],
            }
            done += 1
            job_tracker.tick(job_id, done)
        job_tracker.finish(job_id, {"scores": results})
    except Exception as exc:
        job_tracker.fail(job_id, str(exc))


@router.post("/evaluate")
def evaluate(req: dict, background_tasks: BackgroundTasks):
    """Accuracy of the current bank on a held-out, hand-labeled test set.
    Explicitly triggered -- it re-runs inference over every test image, so it
    runs as a background job with progress rather than blocking the request.

    Job result: overall/per_class precision-recall-F1 plus per_image detail --
    see services/metrics.py::evaluate()."""
    inp = checked_path(req["input_dir"])
    state_dir = str(deps.state_dir(inp))
    _bank_or_400(state_dir)
    try:
        gt = metrics.load_ground_truth_db(str(inp))
    except FileNotFoundError as exc:
        raise HTTPException(400, str(exc))
    conf = req.get("conf", 0.25)
    conf_by_class = req.get("conf_by_class", {})
    job_id = job_tracker.create(len(gt))
    background_tasks.add_task(_run_evaluate, job_id, state_dir, gt, conf, conf_by_class)
    return {"job_id": job_id, "total": len(gt)}


def _run_evaluate(job_id: str, state_dir: str, gt: dict, conf: float,
                  conf_by_class: dict[str, float]):
    try:
        pred = {}
        done = 0
        for line in vpe_client.predict_stream(state_dir, list(gt), conf, conf_by_class):
            if line.get("done"):
                break
            pred[line["image"]] = line["boxes"]
            done += 1
            job_tracker.tick(job_id, done)
        job_tracker.finish(job_id, metrics.evaluate(gt, pred)
                           | {"conf": conf, "conf_by_class": conf_by_class})
    except Exception as exc:
        job_tracker.fail(job_id, str(exc))


@router.post("/autolabel")
def autolabel(req: dict, background_tasks: BackgroundTasks):
    """Write labels for these images straight from the bank. Only worth doing
    once the test-set numbers say the bank is good enough. Runs as a
    background job -- a full pool pass has the same cost as Evaluate.

    Job result: `{"written": int, "no_detection": int, "no_detection_images": [str], "bank": dict}`."""
    inp = checked_path(req["input_dir"])
    state_dir = str(deps.state_dir(inp))
    _bank_or_400(state_dir)
    paths = [str(checked_path(p)) for p in req["images"]]
    conf = req.get("conf", 0.25)
    conf_by_class = req.get("conf_by_class", {})
    job_id = job_tracker.create(len(paths))
    background_tasks.add_task(_run_autolabel, job_id, str(inp), state_dir,
                              paths, conf, conf_by_class)
    return {"job_id": job_id, "total": len(paths)}


def _run_autolabel(job_id: str, input_dir: str, state_dir: str, paths: list[str],
                   conf: float, conf_by_class: dict[str, float]):
    try:
        written, empty, auto_paths = 0, [], []
        done = 0
        for line in vpe_client.predict_stream(state_dir, paths, conf, conf_by_class):
            if line.get("done"):
                break
            if not line["boxes"]:
                # FR-28 -- name the images, not just the count: "12 with nothing
                # found" is a number, a list is something a person can act on.
                empty.append(line["image"])
            else:
                annotations_db.write_boxes(input_dir, "pool", line["image"], line["boxes"])
                written += 1
                auto_paths.append(line["image"])
            done += 1
            job_tracker.tick(job_id, done)
        annotations_db.mark_auto(input_dir, auto_paths)
        job_tracker.finish(job_id, {"written": written, "no_detection": len(empty),
                                    "no_detection_images": empty,
                                    "bank": bank_summary(input_dir, state_dir)})
    except Exception as exc:
        job_tracker.fail(job_id, str(exc))


@router.post("/reembed")
def reembed(req: dict, background_tasks: BackgroundTasks):
    """Switch an already-taught project to a different checkpoint by
    re-extracting every stored instance's embedding under it -- the only
    sanctioned way to change a bank's model after its first label (see
    Bank.lock_model()). Runs as a background job: a bank with hundreds of
    taught instances re-reads and re-infers every one of them. Labels in
    PostgreSQL are never touched -- only the prompt bank's vectors and `model`
    change.

    Job result: `{"bank": dict}`."""
    inp = checked_path(req["input_dir"])
    state_dir = str(deps.state_dir(inp))
    model_id = req["model_id"]
    info = vpe_client.total_instances(state_dir)
    if info["model"] is None:
        raise HTTPException(400, "this project has no model yet -- just label normally, no need to reembed")
    if model_id == info["model"]:
        raise HTTPException(400, f"already using {model_id!r}")
    try:
        model_registry.checkpoint_path(model_id)  # fail fast on a bad id, before spawning the job
    except ValueError as exc:
        raise HTTPException(400, str(exc))
    job_id = job_tracker.create(info["total"])
    background_tasks.add_task(_run_reembed, job_id, str(inp), state_dir, model_id)
    return {"job_id": job_id, "total": info["total"]}


def _run_reembed(job_id: str, input_dir: str, state_dir: str, model_id: str):
    try:
        for line in vpe_client.reembed_stream(state_dir, model_id):
            if line.get("done"):
                break
            job_tracker.tick(job_id, line["done_count"])
        job_tracker.finish(job_id, {"bank": bank_summary(input_dir, state_dir)})
    except Exception as exc:
        job_tracker.fail(job_id, str(exc))
