"""T-23 -- one-time migration of a pre-DB project's file-based labels into
PostgreSQL. Reads the old `.ctflow/classes.txt` + `.ctflow/labels/*.txt`
(pool) and `.ctflow/testset/` (testset.json + its own classes.txt + labels/)
and recreates the same classes (in the same order, so old label indices
still mean what they used to) and annotations as DB rows, plus the
labeled/auto status that used to live in `_bank/metadata.json`.

Does not touch the prompt bank (_bank/embeddings.pt, metadata.json's
instances/model) at all -- those stay file-based, unaffected by this
migration (see docs/DB_MIGRATION_PLAN.md).

    .venv\\Scripts\\python.exe -m backend._migrate_to_db <input_dir> [<input_dir> ...]

Safe to re-run against the same project: write_boxes() replaces an image's
boxes every time, so migrating twice just rewrites the same rows.
"""
import json
import sys
from pathlib import Path

import cv2

from .services import annotations_db, db
from .services.images import list_images


def _read_classes(txt: Path) -> list[str]:
    if not txt.exists():
        return []
    return [n for n in txt.read_text(encoding="utf-8").splitlines() if n.strip()]


def _read_yolo(txt: Path, names: list[str], w: int, h: int) -> list[dict]:
    boxes = []
    for line in txt.read_text(encoding="utf-8").splitlines():
        parts = line.split()
        if len(parts) != 5:
            continue
        k, cx, cy, bw, bh = int(parts[0]), *(float(v) for v in parts[1:])
        boxes.append({
            "cls": names[k] if k < len(names) else str(k),
            "box": [(cx - bw / 2) * w, (cy - bh / 2) * h,
                    (cx + bw / 2) * w, (cy + bh / 2) * h],
        })
    return boxes


def _migrate_kind(input_dir: str, base: Path, kind: str, image_by_stem: dict[str, str]) -> int:
    """`base` is the old on-disk dir that held labels/ + classes.txt for
    this kind (state_dir for pool, state_dir/testset for testset). Returns
    the number of images migrated."""
    names = _read_classes(base / "classes.txt")
    for n in names:  # register in the same order even if a class ends up with zero boxes below
        annotations_db.get_or_create_class(input_dir, kind, n)
    labels_dir = base / "labels"
    if not labels_dir.is_dir():
        return 0
    migrated = 0
    for txt in sorted(labels_dir.glob("*.txt")):
        image_path = image_by_stem.get(txt.stem)
        if image_path is None:
            print(f"  skip {txt.name}: no matching image for stem {txt.stem!r}")
            continue
        img = cv2.imread(image_path)
        if img is None:
            print(f"  skip {txt.name}: image unreadable ({image_path})")
            continue
        h, w = img.shape[:2]
        annotations_db.write_boxes(input_dir, kind, image_path, _read_yolo(txt, names, w, h))
        migrated += 1
    return migrated


def migrate(input_dir: str) -> None:
    inp = Path(input_dir)
    state = inp / ".ctflow"
    if not state.is_dir():
        print(f"{input_dir}: no .ctflow/ -- nothing to migrate")
        return
    by_stem = {Path(p).stem: p for p in list_images(str(inp))}

    print(f"{input_dir}: pool")
    print(f"  {_migrate_kind(input_dir, state, 'pool', by_stem)} image(s) migrated")

    meta_path = state / "_bank" / "metadata.json"
    if meta_path.exists():
        meta = json.loads(meta_path.read_text(encoding="utf-8"))
        for p in meta.get("labeled", []):
            annotations_db.mark_labeled(input_dir, "pool", p)
        annotations_db.mark_auto(input_dir, meta.get("auto", []))
        print(f"  status: {len(meta.get('labeled', []))} labeled, {len(meta.get('auto', []))} auto")

    test_manifest = state / "testset" / "testset.json"
    if test_manifest.exists():
        flagged = json.loads(test_manifest.read_text(encoding="utf-8")).get("images", [])
        annotations_db.mark_test(input_dir, flagged)
        print(f"{input_dir}: testset ({len(flagged)} flagged)")
        print(f"  {_migrate_kind(input_dir, state / 'testset', 'testset', by_stem)} image(s) migrated")


def demo():
    """Self-check: build a fake pre-DB project on disk (old-shape
    .ctflow/labels + classes.txt + testset), migrate it, and confirm the DB
    ends up with the same classes/boxes/status the files described."""
    import shutil
    import tempfile

    import numpy as np

    with tempfile.TemporaryDirectory() as tmp:
        pool = Path(tmp)
        img_a, img_b = pool / "a.jpg", pool / "b.jpg"
        cv2.imwrite(str(img_a), np.zeros((100, 100, 3), np.uint8))
        cv2.imwrite(str(img_b), np.zeros((100, 100, 3), np.uint8))

        state = pool / ".ctflow"
        (state / "labels").mkdir(parents=True)
        (state / "classes.txt").write_text("widget", encoding="utf-8")
        (state / "labels" / "a.txt").write_text("0 0.5 0.5 0.2 0.2", encoding="utf-8")
        (state / "_bank").mkdir()
        (state / "_bank" / "metadata.json").write_text(
            json.dumps({"instances": {}, "labeled": [str(img_a)], "auto": [], "model": None}),
            encoding="utf-8",
        )

        test_dir = state / "testset"
        (test_dir / "labels").mkdir(parents=True)
        (test_dir / "classes.txt").write_text("widget", encoding="utf-8")
        (test_dir / "labels" / "b.txt").write_text("0 0.5 0.5 0.4 0.4", encoding="utf-8")
        (test_dir / "testset.json").write_text(
            json.dumps({"images": [str(img_b)]}), encoding="utf-8"
        )

        db.init_schema()
        annotations_db.delete_project(str(pool))
        migrate(str(pool))

        assert annotations_db.get_classes(str(pool), "pool") == ["widget"]
        pool_boxes = annotations_db.read_boxes(str(pool), "pool", str(img_a))
        assert pool_boxes and pool_boxes[0]["cls"] == "widget", pool_boxes
        assert annotations_db.list_by_status(str(pool), "pool") == {"labeled": [str(img_a)], "auto": []}

        assert annotations_db.is_test(str(pool), str(img_b))
        test_boxes = annotations_db.read_boxes(str(pool), "testset", str(img_b))
        assert test_boxes and test_boxes[0]["cls"] == "widget", test_boxes

        annotations_db.delete_project(str(pool))
    print("migrate_to_db self-check OK")


def main():
    if len(sys.argv) < 2:
        print("usage: python -m backend._migrate_to_db <input_dir> [<input_dir> ...]")
        sys.exit(1)
    db.init_schema()
    for d in sys.argv[1:]:
        migrate(d)


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "--selfcheck":
        demo()
    else:
        main()
