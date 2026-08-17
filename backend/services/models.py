"""Selectable YOLOE checkpoints.

Only the prompt-capable segmentation releases are listed -- the "-pf"
(prompt-free, fixed open-vocabulary) checkpoints can't take set_classes(vpe),
so arm()/extract_embedding() would break on them. Filenames are copied from
ultralytics.utils.downloads.GITHUB_ASSETS_NAMES, which is what actually
resolves an auto-download -- a typo here fails at load time, not at import.
"""
from pathlib import Path

from .. import config

CATALOG = [
    {"id": "yoloe-v8s-seg", "family": "YOLOE v8", "size": "S", "file": "yoloe-v8s-seg.pt",
     "note": "oldest generation, smallest"},
    {"id": "yoloe-v8m-seg", "family": "YOLOE v8", "size": "M", "file": "yoloe-v8m-seg.pt", "note": ""},
    {"id": "yoloe-v8l-seg", "family": "YOLOE v8", "size": "L", "file": "yoloe-v8l-seg.pt", "note": ""},
    {"id": "yoloe-11s-seg", "family": "YOLOE 11", "size": "S", "file": "yoloe-11s-seg.pt",
     "note": "current default"},
    {"id": "yoloe-11m-seg", "family": "YOLOE 11", "size": "M", "file": "yoloe-11m-seg.pt", "note": ""},
    {"id": "yoloe-11l-seg", "family": "YOLOE 11", "size": "L", "file": "yoloe-11l-seg.pt", "note": ""},
    {"id": "yoloe-26n-seg", "family": "YOLOE 26", "size": "N", "file": "yoloe-26n-seg.pt",
     "note": "newest generation, smallest/fastest"},
    {"id": "yoloe-26s-seg", "family": "YOLOE 26", "size": "S", "file": "yoloe-26s-seg.pt", "note": ""},
    {"id": "yoloe-26m-seg", "family": "YOLOE 26", "size": "M", "file": "yoloe-26m-seg.pt", "note": ""},
    {"id": "yoloe-26l-seg", "family": "YOLOE 26", "size": "L", "file": "yoloe-26l-seg.pt", "note": ""},
    {"id": "yoloe-26x-seg", "family": "YOLOE 26", "size": "X", "file": "yoloe-26x-seg.pt",
     "note": "newest generation, largest, best accuracy"},
]
BY_ID = {m["id"]: m for m in CATALOG}
DEFAULT_MODEL_ID = "yoloe-11s-seg"


def checkpoint_path(model_id: str) -> str:
    """Where this checkpoint lives (or lands once ultralytics auto-downloads
    it -- see config.MODELS_DIR)."""
    m = BY_ID.get(model_id)
    if m is None:
        raise ValueError(f"unknown model {model_id!r}")
    return str(Path(config.MODELS_DIR) / m["file"])


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
    try:
        checkpoint_path("not-a-real-model")
        assert False
    except ValueError:
        pass
    print("models self-check OK")


if __name__ == "__main__":
    demo()
