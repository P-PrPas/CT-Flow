"""Selectable YOLOE checkpoints.

The catalog itself lives in backend/models.json, not in this file. Two
processes need it and they are written in different languages: the Python
inference sidecar resolves an id to a weight file to load, and the Go API
serves the list to the frontend and reports which weights are already on disk
(docs/history/REFACTOR_PLAN.md). A shared data file is the only arrangement where
adding a checkpoint is one edit instead of two that can disagree -- and a
disagreement here is not a crash, it is `GET /api/config` advertising a model
the sidecar cannot load.

Only the prompt-capable segmentation releases are listed -- the "-pf"
(prompt-free, fixed open-vocabulary) checkpoints can't take set_classes(vpe),
so arm()/extract_embedding() would break on them. Filenames are copied from
ultralytics.utils.downloads.GITHUB_ASSETS_NAMES, which is what actually
resolves an auto-download -- a typo there fails at load time, not at import.
"""
import json
import os
from pathlib import Path

# Read here rather than from a shared config module: this package is now just
# the inference sidecar, and MODELS_DIR is the only setting it needs. The Go API
# reads the same variable for its own is_available() check.
MODELS_DIR = os.getenv("MODELS_DIR", str(Path(__file__).resolve().parents[3] / "models"))

CATALOG_PATH = Path(__file__).resolve().parent.parent / "models.json"
_raw = json.loads(CATALOG_PATH.read_text(encoding="utf-8"))

# One selectable checkpoint, as returned by GET /api/config -- `id` is what a
# client sends back as `model_id` (POST /api/label, /api/reembed): {id, family,
# size, note, available} where `available` (bool) means the weight is already
# on MODELS_DIR, vs. auto-downloading on first use.
CATALOG: list[dict] = _raw["catalog"]
BY_ID = {m["id"]: m for m in CATALOG}
DEFAULT_MODEL_ID: str = _raw["default"]


def checkpoint_path(model_id: str) -> str:
    """Where this checkpoint lives (or lands once ultralytics auto-downloads
    it -- see MODELS_DIR above)."""
    m = BY_ID.get(model_id)
    if m is None:
        raise ValueError(f"unknown model {model_id!r}")
    return str(Path(MODELS_DIR) / m["file"])


def is_available(model_id: str) -> bool:
    """Whether the checkpoint is already on disk -- picking one that isn't
    means the first predict/label call pays an (auto-download) tax, or fails
    outright with no local internet path to github."""
    return Path(checkpoint_path(model_id)).is_file()


def public_catalog() -> list[dict]:
    """What the frontend gets -- no local file path leaks out."""
    return [
        {**{k: v for k, v in m.items() if k != "file"}, "available": is_available(m["id"])}
        for m in CATALOG
    ]


def demo():
    assert BY_ID[DEFAULT_MODEL_ID]["file"] == "yoloe-11s-seg.pt"
    assert all("file" not in m for m in public_catalog())
    assert all(isinstance(m["available"], bool) for m in public_catalog())
    assert len(public_catalog()) == len(CATALOG)
    # Every entry has to carry the keys both the frontend and the sidecar read;
    # a half-filled row in models.json is the failure this file now invites.
    for m in CATALOG:
        assert {"id", "family", "size", "file", "note"} == set(m), m
        assert m["file"].endswith(".pt"), m
    assert len({m["id"] for m in CATALOG}) == len(CATALOG), "duplicate model id"
    try:
        checkpoint_path("not-a-real-model")
        assert False
    except ValueError:
        pass
    print(f"models self-check OK ({len(CATALOG)} checkpoints from {CATALOG_PATH.name})")


if __name__ == "__main__":
    demo()
