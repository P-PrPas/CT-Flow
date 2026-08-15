"""In-memory progress tracking for long-running endpoints (evaluate, autolabel,
score). A request starts a job and gets a job_id back immediately; the UI
polls GET /api/jobs/{id} for {done, total, ...} to drive a progress bar + ETA.

ponytail: single dict in this process, never pruned -- fine for one uvicorn
worker serving a handful of people; move to Redis (or add TTL eviction) if
this ever runs with >1 worker or sees real traffic.
"""
import threading
import time
import uuid

_jobs: dict[str, dict] = {}
_lock = threading.Lock()


def create(total: int) -> str:
    job_id = uuid.uuid4().hex
    with _lock:
        _jobs[job_id] = {
            "done": 0, "total": total, "started_at": time.time(),
            "finished": False, "result": None, "error": None,
        }
    return job_id


def tick(job_id: str, done: int):
    with _lock:
        if job_id in _jobs:
            _jobs[job_id]["done"] = done


def finish(job_id: str, result):
    with _lock:
        if job_id in _jobs:
            _jobs[job_id]["finished"] = True
            _jobs[job_id]["result"] = result


def fail(job_id: str, error: str):
    with _lock:
        if job_id in _jobs:
            _jobs[job_id]["finished"] = True
            _jobs[job_id]["error"] = error


def get(job_id: str) -> dict | None:
    with _lock:
        j = _jobs.get(job_id)
        return dict(j) if j is not None else None
