"""Evaluate-run history: one point per Evaluate, kept next to the bank it
measured (T-07).

Split out of bank.py because it is the one thing in there that needs no torch.
Leaving it there meant every process that only wants to draw a learning curve
imported the whole ML stack -- and it is what lets the API service drop torch
entirely now that the bank belongs to the inference sidecar
(docs/REFACTOR_PLAN.md).

Lives on disk so the curve survives a browser, a machine, and a colleague.
"""
import json
from pathlib import Path

HISTORY_MAX = 200


def history_path(output_dir: str) -> Path:
    return Path(output_dir) / "_bank" / "eval_history.json"


def read_history(output_dir: str) -> list[dict]:
    """T-07: every Evaluate run, kept next to the bank it measured. Lives on
    disk so the learning curve survives a browser, a machine, and a colleague."""
    p = history_path(output_dir)
    if not p.exists():
        return []
    try:
        return json.loads(p.read_text(encoding="utf-8"))
    except json.JSONDecodeError:
        return []  # a truncated history is a nicety to lose, never an error to raise


def append_history(output_dir: str, point: dict) -> list[dict]:
    # ponytail: read-modify-write, no lock. Two people evaluating the same
    # output_dir in the same second would drop a point -- reuse Bank.lock if
    # that ever stops being hypothetical.
    p = history_path(output_dir)
    p.parent.mkdir(parents=True, exist_ok=True)
    hist = (read_history(output_dir) + [point])[-HISTORY_MAX:]
    p.write_text(json.dumps(hist, ensure_ascii=False), encoding="utf-8")
    return hist
