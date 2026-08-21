"""Ground truth for the held-out test set. Deliberately separate from
pool.py -- these images must never touch the prompt bank, or the F1 read
back from /api/evaluate is measuring memorization instead of generalization.

There is no `POST /session` here: `POST /api/session` (pool.py) already
bundles this state, since the test set lives under the same project folder
as the bank -- one input_dir selection covers both."""
import cv2
from fastapi import APIRouter, HTTPException

from .. import deps
from ..deps import checked_path
from ..services import groundtruth

router = APIRouter(prefix="/api/testset", tags=["testset"])


@router.post("/import")
def import_testset(req: dict):
    """Flag pool images as held-out test set. No file copy -- the pool image
    is the test image, just marked in a manifest, so nothing duplicates an
    image byte."""
    d = str(deps.test_dir(checked_path(req["input_dir"])))
    marked = groundtruth.mark_test(d, [str(checked_path(p)) for p in req["images"]])
    return {
        "images": groundtruth.list_test_images(d),
        "labeled": sorted(groundtruth.labeled_stems(d)),
        "classes": groundtruth.load_classes(d),
        "imported": marked,
    }


@router.post("/remove")
def remove_testset(req: dict):
    """Unflag images out of the test set (manifest entry + ground truth). The
    pool image itself is untouched -- there is no copy to un-delete."""
    d = str(deps.test_dir(checked_path(req["input_dir"])))
    removed = groundtruth.unmark_test(d, [str(checked_path(p)) for p in req["images"]])
    return {
        "images": groundtruth.list_test_images(d),
        "labeled": sorted(groundtruth.labeled_stems(d)),
        "classes": groundtruth.load_classes(d),
        "removed": removed,
    }


@router.post("/label")
def label_testset(req: dict):
    """Writes ground truth for one test-set image. No embedding extraction,
    no bank interaction whatsoever -- this is the one write path in the
    whole app that's fully independent of Bank. `400` if `image` isn't
    flagged in the test set -- Import it first (POST /api/testset/import)."""
    inp = checked_path(req["input_dir"])
    d = str(deps.test_dir(inp))
    if not groundtruth.is_test(d, req["image"]):
        raise HTTPException(400, "this image isn't in the test set yet -- import it first")
    img = cv2.imread(str(checked_path(req["image"])))
    if img is None:
        raise HTTPException(400, "cannot read image")
    boxes = req["boxes"]
    if not boxes:
        raise HTTPException(400, "no boxes")
    h, w = img.shape[:2]
    names = groundtruth.write_label(
        d, req["image"], boxes, w, h, merge=(req.get("mode", "replace") == "update"),
    )
    return {"classes": names, "labeled": sorted(groundtruth.labeled_stems(d))}
