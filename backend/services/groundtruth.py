"""YOLO ground truth for a test-set folder, independent of the prompt bank --
test images are held out and must never be fed to the model as prompts, so
this never touches embeddings.py or bank.py.

    <test_dir>/*.jpg          the held-out images (put there by hand)
    <test_dir>/labels/*.txt   YOLO ground truth, written here
    <test_dir>/classes.txt    class index -> name, grown as new names appear
"""
import shutil
from pathlib import Path

from . import yolo_labels
from .images import list_images


def import_images(test_dir: str, source_paths: list[str]) -> list[str]:
    """Copy pool images into the test-set folder by filename. A real copy, not
    a reference -- the pool keeps its own file so nothing there changes, and
    the test set becomes a self-contained folder you can hand off. Skips
    names already present so re-importing never clobbers existing ground
    truth. Returns the paths actually copied."""
    d = Path(test_dir)
    d.mkdir(parents=True, exist_ok=True)
    copied = []
    for src in source_paths:
        src = Path(src)
        dst = d / src.name
        if dst.exists() or not src.is_file():
            continue
        shutil.copy2(src, dst)
        copied.append(str(dst))
    return copied


def remove_images(test_dir: str, image_paths: list[str]) -> list[str]:
    """Delete images (and their ground-truth label, if any) out of the test
    set. Only touches files inside test_dir -- if a path resolves elsewhere
    (e.g. still points at the pool) it's skipped, since this must never
    delete the source pool image. classes.txt is left as-is: reindexing it
    would require rewriting every remaining label file's class column, for
    no real benefit over one unused name sitting in the list. Returns the
    stems actually removed."""
    d = Path(test_dir).resolve()
    removed = []
    for src in image_paths:
        p = Path(src)
        try:
            p.resolve().relative_to(d)
        except ValueError:
            continue
        if not p.is_file():
            continue
        stem = p.stem
        p.unlink()
        label = d / "labels" / f"{stem}.txt"
        if label.exists():
            label.unlink()
        removed.append(stem)
    return removed


def load_classes(test_dir: str) -> list[str]:
    f = Path(test_dir) / "classes.txt"
    if not f.exists():
        return []
    return [n for n in f.read_text(encoding="utf-8").splitlines() if n.strip()]


def labeled_stems(test_dir: str) -> set[str]:
    d = Path(test_dir) / "labels"
    return {f.stem for f in d.glob("*.txt")} if d.exists() else set()


def read_boxes(dir_path: str, image_path: str) -> list[dict]:
    """Read a YOLO label file back into pixel-coord boxes. Works on any folder
    following the labels/ + classes.txt convention -- a pool's output_dir
    (written by bank.write_yolo_labels) or a test_dir (written by
    write_label below) look the same on disk. [] if the image has no label
    file yet (nothing to show)."""
    return yolo_labels.read_boxes(dir_path, image_path, load_classes(dir_path))


def write_label(test_dir: str, image_path: str, boxes: list[dict],
                 width: int, height: int, merge: bool = False) -> list[str]:
    """boxes: [{"cls": name, "box": [x1,y1,x2,y2]}] in pixel coords. New class
    names are appended (never reordered, so earlier labels' indices stay
    valid). By default this replaces the image's ground truth entirely;
    merge=True reads what's already saved first and writes the union, for
    adding a box without retyping the rest. Returns the resulting class
    list."""
    names = load_classes(test_dir)
    if merge:
        boxes = yolo_labels.read_boxes(test_dir, image_path, names) + boxes
    for b in boxes:
        if b["cls"] not in names:
            names.append(b["cls"])
    yolo_labels.write_boxes(test_dir, image_path, boxes, width, height, names)
    (Path(test_dir) / "classes.txt").write_text("\n".join(names), encoding="utf-8")
    return names
