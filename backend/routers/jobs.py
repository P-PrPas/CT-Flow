"""Long inference passes over a folder (score/evaluate/autolabel/reembed) all
run as a background job: the request returns a job_id immediately, and the UI
polls GET /api/jobs/{id} for progress. See services/job_tracker.py.

Every job-starting endpoint below returns the same shape ({job_id, total}) and
is polled the same way; only `result`'s shape once `finished` differs per job
-- documented on each endpoint since GET /api/jobs/{id} itself can't know
which kind of job it's looking at."""
import time

import cv2
from fastapi import APIRouter, BackgroundTasks, HTTPException

from .. import deps
from ..deps import checked_path
from ..services import annotations_db, job_tracker, metrics
from ..services import models as model_registry
from ..services.bank import Bank
from ..services.vpe import arm, extract_embedding, predict_one

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


@router.post("/score")
def score(req: dict, background_tasks: BackgroundTasks):
    """Rescore the pool with the current bank. Runs as a background job so
    the UI can show progress -- a few hundred images takes real time.

    Job result: `{"scores": {image_path: {"conf": float, "cls": str|None, "sig": [int, ...]}}}`."""
    paths = [str(checked_path(p)) for p in req["images"]]
    job_id = job_tracker.create(len(paths))
    bank = Bank(str(deps.state_dir(checked_path(req["input_dir"]))))
    mean = bank.mean_vpe()
    if mean is None or not paths:
        job_tracker.finish(job_id, {"scores": {}})
        return {"job_id": job_id, "total": 0}
    names, combined = mean
    background_tasks.add_task(_run_score, job_id, names, combined, paths, bank.model_or_default)
    return {"job_id": job_id, "total": len(paths)}


def _signature(path: str) -> list[int]:
    """FR-18 — an 8x8 grayscale thumbnail, so the UI can tell "another frame of
    the same thing" from "something new" and stop offering five near-identical
    images in a row. ponytail: a perceptual thumbnail, not bank-embedding
    distance; it catches conveyor near-duplicates, which is the actual
    complaint. Swap in embeddings if it ever misses a case that matters."""
    img = cv2.imread(path, cv2.IMREAD_GRAYSCALE)
    if img is None:
        return []
    return cv2.resize(img, (8, 8), interpolation=cv2.INTER_AREA).flatten().tolist()


def _run_score(job_id: str, names: list[str], combined, paths: list[str], model_id: str):
    try:
        model = arm(names, combined, model_id)
        results = {}
        for i, path in enumerate(paths):
            dets = predict_one(model, names, path, conf=0.05)
            best = max(dets, key=lambda d: d["conf"], default=None)
            results[path] = {
                "conf": best["conf"] if best else 0.0,
                "cls": best["cls"] if best else None,
                "sig": _signature(path),
            }
            job_tracker.tick(job_id, i + 1)
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
    bank = Bank(str(deps.state_dir(inp)))
    mean = bank.mean_vpe()
    if mean is None:
        raise HTTPException(400, "prompt bank is empty -- label something first")
    try:
        gt = metrics.load_ground_truth_db(str(inp))
    except FileNotFoundError as exc:
        raise HTTPException(400, str(exc))
    names, combined = mean
    conf = req.get("conf", 0.25)
    conf_by_class = req.get("conf_by_class", {})
    job_id = job_tracker.create(len(gt))
    background_tasks.add_task(_run_evaluate, job_id, names, combined, gt,
                              conf, conf_by_class, bank.model_or_default)
    return {"job_id": job_id, "total": len(gt)}


def _run_evaluate(job_id: str, names: list[str], combined, gt: dict, conf: float,
                  conf_by_class: dict[str, float], model_id: str):
    try:
        model = arm(names, combined, model_id)
        pred = {}
        for i, path in enumerate(gt):
            pred[path] = predict_one(model, names, path, conf, conf_by_class)
            job_tracker.tick(job_id, i + 1)
        job_tracker.finish(job_id, metrics.evaluate(gt, pred)
                           | {"conf": conf, "conf_by_class": conf_by_class})
    except Exception as exc:
        job_tracker.fail(job_id, str(exc))


@router.post("/autolabel")
def autolabel(req: dict, background_tasks: BackgroundTasks):
    """Write YOLO labels for these images straight from the bank. Only worth
    doing once the test-set numbers say the bank is good enough. Runs as a
    background job -- a full pool pass has the same cost as Evaluate.

    Job result: `{"written": int, "no_detection": int, "no_detection_images": [str], "bank": dict}`."""
    inp = checked_path(req["input_dir"])
    state_dir = str(deps.state_dir(inp))
    bank = Bank(state_dir)
    mean = bank.mean_vpe()
    if mean is None:
        raise HTTPException(400, "prompt bank is empty -- label something first")
    names, combined = mean
    paths = [str(checked_path(p)) for p in req["images"]]
    conf = req.get("conf", 0.25)
    conf_by_class = req.get("conf_by_class", {})
    job_id = job_tracker.create(len(paths))
    background_tasks.add_task(_run_autolabel, job_id, str(inp), state_dir,
                              names, combined, paths, conf, conf_by_class, bank.model_or_default)
    return {"job_id": job_id, "total": len(paths)}


def _run_autolabel(job_id: str, input_dir: str, output_dir: str, names: list[str], combined,
                   paths: list[str], conf: float, conf_by_class: dict[str, float], model_id: str):
    try:
        bank = Bank(output_dir)
        model = arm(names, combined, model_id)
        written, empty, auto_paths = 0, [], []
        for i, path in enumerate(paths):
            dets = predict_one(model, names, path, conf, conf_by_class)
            if not dets:
                # FR-28 -- name the images, not just the count: "12 with nothing
                # found" is a number, a list is something a person can act on.
                empty.append(path)
            else:
                annotations_db.write_boxes(input_dir, "pool", path, dets)
                written += 1
                auto_paths.append(path)
            job_tracker.tick(job_id, i + 1)
        annotations_db.mark_auto(input_dir, auto_paths)
        job_tracker.finish(job_id, {"written": written, "no_detection": len(empty),
                                    "no_detection_images": empty, "bank": bank.summary()})
    except Exception as exc:
        job_tracker.fail(job_id, str(exc))


@router.post("/reembed")
def reembed(req: dict, background_tasks: BackgroundTasks):
    """Switch an already-taught project to a different checkpoint by
    re-extracting every stored instance's embedding under it -- the only
    sanctioned way to change a bank's model after its first label (see
    Bank.lock_model()). Runs as a background job: a bank with hundreds of
    taught instances re-reads and re-infers every one of them. Label files
    are never touched -- only the prompt bank's vectors and `model` change.

    Job result: `{"bank": dict}`."""
    state_dir = str(deps.state_dir(checked_path(req["input_dir"])))
    bank = Bank(state_dir)
    model_id = req["model_id"]
    if bank.model is None:
        raise HTTPException(400, "this project has no model yet -- just label normally, no need to reembed")
    if model_id == bank.model:
        raise HTTPException(400, f"already using {model_id!r}")
    try:
        model_registry.checkpoint_path(model_id)  # fail fast on a bad id, before spawning the job
    except ValueError as exc:
        raise HTTPException(400, str(exc))
    total = sum(len(insts) for insts in bank.instances.values())
    job_id = job_tracker.create(total)
    background_tasks.add_task(_run_reembed, job_id, state_dir, model_id)
    return {"job_id": job_id, "total": total}


def _run_reembed(job_id: str, output_dir: str, model_id: str):
    try:
        bank = Bank(output_dir)
        new_embeddings: dict[str, list] = {}
        done = 0
        for cls_name in bank.classes:
            vecs = []
            for inst in bank.instances[cls_name]:
                img = cv2.imread(inst["source_image"])
                if img is None:
                    raise ValueError(f"cannot re-read {inst['source_image']!r} -- has it moved or been deleted?")
                # ponytail: an instance taught from several same-class boxes in
                # one save is stored with only the first box's coordinates
                # (see routers/pool.py::save_label) -- reembed can only replay
                # that one box, so a multi-box instance loses the averaging it
                # originally had. Rare in practice (most saves are one box per
                # class); store the full box list per instance if this turns
                # out to matter.
                vecs.append(extract_embedding(img, [inst["bbox"]], model_id))
                done += 1
                job_tracker.tick(job_id, done)
            new_embeddings[cls_name] = vecs
        bank.reembed(model_id, new_embeddings)
        job_tracker.finish(job_id, {"bank": bank.summary()})
    except Exception as exc:
        job_tracker.fail(job_id, str(exc))
