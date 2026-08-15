"""The core labeling loop: open an image pool, save boxes into the prompt
bank, review/fix a generated label without touching the bank."""
from typing import Literal

import cv2
from fastapi import APIRouter, Depends, HTTPException
from fastapi.responses import FileResponse
from pydantic import BaseModel

from ..deps import checked_path, current_user
from ..services import bank as bank_store
from ..services import events as event_log
from ..services import groundtruth
from ..services.bank import Bank
from ..services.images import list_images
from ..services.vpe import arm, extract_embedding, predict_one

router = APIRouter(prefix="/api", tags=["pool"])


class Box(BaseModel):
    cls: str
    box: list[float]  # [x1, y1, x2, y2] in source-image pixels


class SessionReq(BaseModel):
    input_dir: str
    output_dir: str


@router.post("/session")
def open_session(req: SessionReq):
    inp, out = checked_path(req.input_dir), checked_path(req.output_dir)
    if not inp.is_dir():
        raise HTTPException(400, f"input dir not found: {inp}")
    out.mkdir(parents=True, exist_ok=True)
    images = list_images(str(inp))
    if not images:
        raise HTTPException(400, f"no images in {inp}")
    bank = Bank(str(out))
    return {"images": images, "bank": bank.summary()}


@router.get("/image")
def get_image(path: str):
    p = checked_path(path)
    if not p.is_file():
        raise HTTPException(404, "image not found")
    return FileResponse(p)


@router.get("/boxes")
def get_boxes(dir: str, image: str):
    """Boxes already saved for this image, so revisiting it shows what's
    there instead of a blank canvas. Works for a pool's output_dir or a
    test_dir -- both use the labels/ + classes.txt layout."""
    d = checked_path(dir)
    img = checked_path(image)
    return {"boxes": groundtruth.read_boxes(str(d), str(img))}


class LabelReq(BaseModel):
    output_dir: str
    image: str
    boxes: list[Box]
    # replace: this image's label file becomes exactly `boxes`.
    # update: `boxes` are added to whatever's already saved for this image.
    mode: Literal["replace", "update"] = "replace"


@router.post("/label")
def save_label(req: LabelReq, user: str | None = Depends(current_user)):
    img = cv2.imread(str(checked_path(req.image)))
    if img is None:
        raise HTTPException(400, "cannot read image")
    if not req.boxes:
        raise HTTPException(400, "no boxes")
    bank = Bank(str(checked_path(req.output_dir)))

    by_class: dict[str, list[list[float]]] = {}
    for b in req.boxes:
        by_class.setdefault(b.cls, []).append(b.box)
    for cls_name, boxes in by_class.items():
        bank.add(cls_name, extract_embedding(img, boxes), req.image, boxes[0], user)
    bank.mark_labeled(req.image)

    h, w = img.shape[:2]
    bank.write_yolo_labels(req.image, [b.model_dump() for b in req.boxes], w, h,
                           merge=(req.mode == "update"))
    return {"bank": bank.summary()}


class PredictReq(BaseModel):
    output_dir: str
    image: str
    conf: float = 0.25
    conf_by_class: dict[str, float] = {}


@router.post("/predict")
def predict(req: PredictReq):
    """FR-19 / T-05 — the model's guesses for ONE image, so the user corrects
    instead of drawing from scratch. Empty bank -> empty list, no forward pass.

    ponytail: synchronous, and arm() mutates the process-wide model, same as
    every other inference path here. Fine for one image and one labeler; give
    predict its own model instance if a second concurrent user shows up."""
    bank = Bank(str(checked_path(req.output_dir)))
    mean = bank.mean_vpe()
    if mean is None:
        return {"boxes": []}
    names, combined = mean
    model = arm(names, combined)
    dets = predict_one(model, names, str(checked_path(req.image)), req.conf, req.conf_by_class)
    return {"boxes": dets}


class HistoryAppendReq(BaseModel):
    output_dir: str
    point: dict


@router.get("/history")
def get_history(output_dir: str):
    return {"history": bank_store.read_history(str(checked_path(output_dir)))}


@router.post("/history")
def add_history(req: HistoryAppendReq):
    return {"history": bank_store.append_history(str(checked_path(req.output_dir)), req.point)}


@router.delete("/history")
def del_history(output_dir: str):
    bank_store.history_path(str(checked_path(output_dir))).unlink(missing_ok=True)
    return {"history": []}


class EventReq(BaseModel):
    output_dir: str
    kind: str            # session | label | fix | auto -- see services/events.py
    session: str = ""    # a browser-side id, so abandonment can be counted
    secs: float | None = None
    written: int = 0


@router.post("/events")
def add_event(req: EventReq, user: str | None = Depends(current_user)):
    """§7 — record one thing that happened, so "does this tool save time" has
    an answer that outlives the tab. Fire-and-forget: the UI never waits on it
    and never shows an error for it."""
    event_log.append(str(checked_path(req.output_dir)),
                     req.model_dump(exclude={"output_dir"}) | {"user": user})
    return {"ok": True}


@router.get("/events")
def get_events(output_dir: str):
    return {"summary": event_log.summary(str(checked_path(output_dir)))}


class RelabelReq(BaseModel):
    output_dir: str
    image: str
    boxes: list[Box]
    mode: Literal["replace", "update"] = "replace"


@router.post("/relabel")
def relabel(req: RelabelReq):
    """Rewrite this image's YOLO label file directly -- no embedding
    extraction, no bank.add, no mark_labeled. For fixing generated labels
    (delete an over-prediction, drag in a box the model missed) without the
    correction being treated as a new visual prompt. `boxes` may be empty --
    that's a legitimate "the model was wrong about everything here"."""
    bank = Bank(str(checked_path(req.output_dir)))
    unknown = {b.cls for b in req.boxes} - set(bank.classes)
    if unknown:
        raise HTTPException(
            400, f"unknown class(es) {sorted(unknown)} -- use Save to bank to teach a new class"
        )
    img = cv2.imread(str(checked_path(req.image)))
    if img is None:
        raise HTTPException(400, "cannot read image")
    h, w = img.shape[:2]
    bank.write_yolo_labels(req.image, [b.model_dump() for b in req.boxes], w, h,
                           merge=(req.mode == "update"))
    return {"bank": bank.summary()}
