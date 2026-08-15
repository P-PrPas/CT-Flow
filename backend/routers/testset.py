"""Ground truth for the held-out test set. Deliberately separate from
pool.py -- these images must never touch the prompt bank, or the F1 read
back from /api/evaluate is measuring memorization instead of generalization."""
from typing import Literal

import cv2
from fastapi import APIRouter, HTTPException
from pydantic import BaseModel

from ..deps import checked_path
from ..services import groundtruth
from .pool import Box

router = APIRouter(prefix="/api/testset", tags=["testset"])


class TestSessionReq(BaseModel):
    test_dir: str


@router.post("/session")
def open_testset(req: TestSessionReq):
    d = checked_path(req.test_dir)
    d.mkdir(parents=True, exist_ok=True)
    images = groundtruth.list_images(str(d))
    return {
        "images": images,
        "labeled": sorted(groundtruth.labeled_stems(str(d))),
        "classes": groundtruth.load_classes(str(d)),
    }


class TestImportReq(BaseModel):
    test_dir: str
    images: list[str]


@router.post("/import")
def import_testset(req: TestImportReq):
    """Copy the given pool images into the test-set folder so they can be
    labeled as ground truth. Copies, not moves -- the pool is untouched."""
    d = checked_path(req.test_dir)
    copied = groundtruth.import_images(str(d), [str(checked_path(p)) for p in req.images])
    return {
        "images": groundtruth.list_images(str(d)),
        "labeled": sorted(groundtruth.labeled_stems(str(d))),
        "classes": groundtruth.load_classes(str(d)),
        "imported": copied,
    }


class TestRemoveReq(BaseModel):
    test_dir: str
    images: list[str]


@router.post("/remove")
def remove_testset(req: TestRemoveReq):
    """Drop images out of the test set (file + ground truth). The pool copy
    these came from, if any, is untouched -- import copies, this un-copies."""
    d = checked_path(req.test_dir)
    removed = groundtruth.remove_images(str(d), [str(checked_path(p)) for p in req.images])
    return {
        "images": groundtruth.list_images(str(d)),
        "labeled": sorted(groundtruth.labeled_stems(str(d))),
        "classes": groundtruth.load_classes(str(d)),
        "removed": removed,
    }


class TestLabelReq(BaseModel):
    test_dir: str
    image: str
    boxes: list[Box]
    mode: Literal["replace", "update"] = "replace"


@router.post("/label")
def label_testset(req: TestLabelReq):
    img = cv2.imread(str(checked_path(req.image)))
    if img is None:
        raise HTTPException(400, "cannot read image")
    if not req.boxes:
        raise HTTPException(400, "no boxes")
    h, w = img.shape[:2]
    names = groundtruth.write_label(
        str(checked_path(req.test_dir)), req.image,
        [b.model_dump() for b in req.boxes], w, h,
        merge=(req.mode == "update"),
    )
    return {"classes": names, "labeled": sorted(groundtruth.labeled_stems(str(checked_path(req.test_dir))))}
