"""Listing images in a folder -- shared by the pool (bank.py's output_dir /
the router's input_dir) and the test set (groundtruth.py), since both are
just "images directly inside this directory, matching IMAGE_EXTS"."""
from pathlib import Path

from ..config import IMAGE_EXTS


def list_images(dir_path: str) -> list[str]:
    p = Path(dir_path)
    if not dir_path or not p.is_dir():
        return []
    return sorted(str(f) for f in p.iterdir() if f.suffix.lower() in IMAGE_EXTS)
