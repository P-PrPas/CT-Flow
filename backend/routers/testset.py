"""Ground truth for the held-out test set. Deliberately separate from
pool.py -- these images must never touch the prompt bank, or the F1 read
back from /api/evaluate is measuring memorization instead of generalization.

There is no `POST /session` here: `POST /api/session` (pool.py) already
bundles this state, since the test set is keyed by the same input_dir as
the bank -- one input_dir selection covers both. Storage is PostgreSQL
(services/annotations_db.py, kind='testset') -- see docs/DB_MIGRATION_PLAN.md."""
import cv2
from fastapi import APIRouter, HTTPException

from ..deps import checked_path
from ..services import annotations_db

router = APIRouter(prefix="/api/testset", tags=["testset"])


@router.post("/import")
def import_testset(req: dict):
    """Flag pool images as held-out test set. No file/row copy -- the pool
    image is the test image, just a second `images` row sharing the same
    path, so nothing duplicates an image byte."""
    inp = str(checked_path(req["input_dir"]))
    marked = annotations_db.mark_test(inp, [str(checked_path(p)) for p in req["images"]])
    return {
        "images": annotations_db.list_test_images(inp),
        "labeled": sorted(annotations_db.labeled_stems(inp)),
        "classes": annotations_db.get_classes(inp, "testset"),
        "imported": marked,
    }


@router.post("/remove")
def remove_testset(req: dict):
    """Unflag images out of the test set (row + ground truth, cascade). The
    pool image itself is untouched -- there is no copy to un-delete."""
    inp = str(checked_path(req["input_dir"]))
    removed = annotations_db.unmark_test(inp, [str(checked_path(p)) for p in req["images"]])
    return {
        "images": annotations_db.list_test_images(inp),
        "labeled": sorted(annotations_db.labeled_stems(inp)),
        "classes": annotations_db.get_classes(inp, "testset"),
        "removed": removed,
    }


@router.post("/label")
def label_testset(req: dict):
    """Writes ground truth for one test-set image. No embedding extraction,
    no bank interaction whatsoever -- this is the one write path in the
    whole app that's fully independent of Bank. `400` if `image` isn't
    flagged in the test set -- Import it first (POST /api/testset/import)."""
    inp = str(checked_path(req["input_dir"]))
    if not annotations_db.is_test(inp, req["image"]):
        raise HTTPException(400, "this image isn't in the test set yet -- import it first")
    if cv2.imread(str(checked_path(req["image"]))) is None:
        raise HTTPException(400, "cannot read image")
    boxes = req["boxes"]
    if not boxes:
        raise HTTPException(400, "no boxes")
    names = annotations_db.write_boxes(
        inp, "testset", req["image"], boxes, merge=(req.get("mode", "replace") == "update"),
    )
    return {"classes": names, "labeled": sorted(annotations_db.labeled_stems(inp))}
