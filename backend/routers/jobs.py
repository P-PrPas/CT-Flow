"""Long inference passes over a folder (score/evaluate/autolabel) all run as
a background job: the request returns a job_id immediately, and the UI polls
GET /api/jobs/{id} for progress. See services/job_tracker.py."""
import time

import cv2
from fastapi import APIRouter, BackgroundTasks, HTTPException
from pydantic import BaseModel

from ..deps import checked_path
from ..services import job_tracker, metrics
from ..services.bank import Bank
from ..services.vpe import arm, predict_one

router = APIRouter(prefix="/api", tags=["jobs"])


@router.get("/jobs/{job_id}")
def get_job(job_id: str):
    j = job_tracker.get(job_id)
    if j is None:
        raise HTTPException(404, "unknown job")
    return j | {"now": time.time()}


class ScoreReq(BaseModel):
    output_dir: str
    images: list[str]


@router.post("/score")
def score(req: ScoreReq, background_tasks: BackgroundTasks):
    """Rescore the pool with the current bank. Runs as a background job so
    the UI can show progress -- a few hundred images takes real time."""
    paths = [str(checked_path(p)) for p in req.images]
    job_id = job_tracker.create(len(paths))
    bank = Bank(str(checked_path(req.output_dir)))
    mean = bank.mean_vpe()
    if mean is None or not paths:
        job_tracker.finish(job_id, {"scores": {}})
        return {"job_id": job_id, "total": 0}
    names, combined = mean
    background_tasks.add_task(_run_score, job_id, names, combined, paths, bank.model)
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


class EvalReq(BaseModel):
    output_dir: str
    test_dir: str
    conf: float = 0.25
    # FR-33: per-class overrides, {} = one threshold for everything (as before).
    conf_by_class: dict[str, float] = {}


@router.post("/evaluate")
def evaluate(req: EvalReq, background_tasks: BackgroundTasks):
    """Accuracy of the current bank on a held-out, hand-labeled test set.
    Explicitly triggered -- it re-runs inference over every test image, so it
    runs as a background job with progress rather than blocking the request."""
    bank = Bank(str(checked_path(req.output_dir)))
    mean = bank.mean_vpe()
    if mean is None:
        raise HTTPException(400, "prompt bank is empty -- label something first")
    try:
        gt = metrics.load_ground_truth(str(checked_path(req.test_dir)))
    except FileNotFoundError as exc:
        raise HTTPException(400, str(exc))
    names, combined = mean
    job_id = job_tracker.create(len(gt))
    background_tasks.add_task(_run_evaluate, job_id, names, combined, gt,
                              req.conf, req.conf_by_class, bank.model)
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


class AutoReq(BaseModel):
    output_dir: str
    images: list[str]
    conf: float = 0.25
    conf_by_class: dict[str, float] = {}


@router.post("/autolabel")
def autolabel(req: AutoReq, background_tasks: BackgroundTasks):
    """Write YOLO labels for these images straight from the bank. Only worth
    doing once the test-set numbers say the bank is good enough. Runs as a
    background job -- a full pool pass has the same cost as Evaluate."""
    bank = Bank(str(checked_path(req.output_dir)))
    mean = bank.mean_vpe()
    if mean is None:
        raise HTTPException(400, "prompt bank is empty -- label something first")
    names, combined = mean
    paths = [str(checked_path(p)) for p in req.images]
    job_id = job_tracker.create(len(paths))
    background_tasks.add_task(_run_autolabel, job_id, str(checked_path(req.output_dir)),
                              names, combined, paths, req.conf, req.conf_by_class, bank.model)
    return {"job_id": job_id, "total": len(paths)}


def _run_autolabel(job_id: str, output_dir: str, names: list[str], combined, paths: list[str],
                   conf: float, conf_by_class: dict[str, float], model_id: str):
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
                img = cv2.imread(path)
                h, w = img.shape[:2]
                bank.write_yolo_labels(path, dets, w, h)
                written += 1
                auto_paths.append(path)
            job_tracker.tick(job_id, i + 1)
        bank.mark_auto(auto_paths)
        job_tracker.finish(job_id, {"written": written, "no_detection": len(empty),
                                    "no_detection_images": empty, "bank": bank.summary()})
    except Exception as exc:
        job_tracker.fail(job_id, str(exc))
