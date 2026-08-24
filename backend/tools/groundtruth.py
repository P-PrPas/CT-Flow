"""Ground truth for the held-out test set. Deliberately separate from
bank.py -- these images must never touch the prompt bank, or the F1 read
back from /api/evaluate is measuring memorization instead of generalization.

Test images are never copied: a test set is just pool images flagged in a
manifest, so the pool and the test set always point at exactly one file per
image on disk.

    <test_dir>/testset.json    {"images": [pool image path, ...]}
    <test_dir>/labels/*.txt    YOLO ground truth, written here
    <test_dir>/classes.txt     class index -> name, grown as new names appear
"""
import json
from pathlib import Path

from . import yolo_labels


def manifest_path(test_dir: str) -> Path:
    return Path(test_dir) / "testset.json"


def list_test_images(test_dir: str) -> list[str]:
    p = manifest_path(test_dir)
    if not p.exists():
        return []
    try:
        return sorted(json.loads(p.read_text(encoding="utf-8")).get("images", []))
    except json.JSONDecodeError:
        return []  # a truncated manifest is a nicety to lose, never an error to raise


def _save_manifest(test_dir: str, images: set[str]) -> None:
    d = Path(test_dir)
    d.mkdir(parents=True, exist_ok=True)
    manifest_path(test_dir).write_text(
        json.dumps({"images": sorted(images)}, ensure_ascii=False), encoding="utf-8"
    )


def mark_test(test_dir: str, source_paths: list[str]) -> list[str]:
    """Flag pool images as held-out test set -- no file copy, so the pool and
    the test set share the exact same file on disk instead of duplicating it.
    Skips paths already flagged. Returns the paths actually added."""
    current = set(list_test_images(test_dir))
    added = [str(Path(p)) for p in source_paths if str(Path(p)) not in current]
    if added:
        _save_manifest(test_dir, current | set(added))
    return added


def unmark_test(test_dir: str, image_paths: list[str]) -> list[str]:
    """Drop images out of the test set: manifest entry + ground truth. The
    pool image itself is never touched -- there is no separate copy of it to
    delete. Returns the paths actually removed."""
    current = set(list_test_images(test_dir))
    drop = {str(Path(p)) for p in image_paths}
    removed = sorted(current & drop)
    if removed:
        _save_manifest(test_dir, current - drop)
        for p in removed:
            (Path(test_dir) / "labels" / f"{Path(p).stem}.txt").unlink(missing_ok=True)
    return removed


def is_test(test_dir: str, image_path: str) -> bool:
    """Whether `image_path` is held out -- pool endpoints that would teach the
    bank must check this and refuse, or a test image silently stops measuring
    generalization."""
    return str(Path(image_path)) in set(list_test_images(test_dir))


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
    following the labels/ + classes.txt convention -- a pool's state_dir
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


def demo():
    import tempfile

    import numpy as np
    import cv2

    with tempfile.TemporaryDirectory() as tmp:
        pool_img = Path(tmp) / "a.jpg"
        cv2.imwrite(str(pool_img), np.zeros((20, 20, 3), np.uint8))
        test_dir = str(Path(tmp) / ".ctflow" / "testset")

        assert mark_test(test_dir, [str(pool_img)]) == [str(pool_img)]
        assert mark_test(test_dir, [str(pool_img)]) == []  # already flagged -> no-op
        assert is_test(test_dir, str(pool_img))
        assert list_test_images(test_dir) == [str(pool_img)]

        write_label(test_dir, str(pool_img), [{"cls": "x", "box": [0, 0, 10, 10]}], 20, 20)
        assert labeled_stems(test_dir) == {"a"}
        assert read_boxes(test_dir, str(pool_img))[0]["cls"] == "x"

        assert unmark_test(test_dir, [str(pool_img)]) == [str(pool_img)]
        assert not is_test(test_dir, str(pool_img))
        assert labeled_stems(test_dir) == set()  # unmarking drops the ground truth too
        assert pool_img.exists()  # the pool image itself is never touched
    print("groundtruth self-check OK")


if __name__ == "__main__":
    demo()
