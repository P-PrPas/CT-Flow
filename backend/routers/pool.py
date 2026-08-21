"""The core labeling loop: open an image pool, save boxes into the prompt
bank, review/fix a generated label without touching the bank."""
import cv2
from fastapi import APIRouter, Depends, HTTPException
from fastapi.responses import FileResponse

from .. import deps
from ..deps import checked_path, current_user
from ..services import bank as bank_store
from ..services import events as event_log
from ..services import groundtruth
from ..services import models as model_registry
from ..services.bank import Bank
from ..services.images import list_images
from ..services.vpe import arm, extract_embedding, predict_one

router = APIRouter(prefix="/api", tags=["pool"])


@router.post("/session")
def open_session(req: dict):
    """Opens the one folder a project needs: the pool. Labels, the prompt
    bank, and the test-set manifest all live in a fixed subfolder of it (see
    deps.state_dir/test_dir) -- nothing else to browse for. Bundles the
    test-set state in the same response so the UI never has to make a second
    "did you forget the test set" round trip."""
    inp = checked_path(req["input_dir"])
    if not inp.is_dir():
        raise HTTPException(400, f"input dir not found: {inp}")
    images = list_images(str(inp))
    if not images:
        raise HTTPException(400, f"no images in {inp}")
    bank = Bank(str(deps.state_dir(inp)))
    td = str(deps.test_dir(inp))
    return {
        "images": images,
        "bank": bank.summary(),
        "testset": {
            "images": groundtruth.list_test_images(td),
            "labeled": sorted(groundtruth.labeled_stems(td)),
            "classes": groundtruth.load_classes(td),
        },
    }


@router.get("/image")
def get_image(path: str):
    """Streams the raw image file at `path`. Response is `image/*`, not
    JSON -- what the browser's `<img src>` and every OpenCV read in this
    backend actually point at."""
    p = checked_path(path)
    if not p.is_file():
        raise HTTPException(404, "image not found")
    return FileResponse(p)


@router.get("/boxes")
def get_boxes(input_dir: str, image: str, kind: str = "pool"):
    """Boxes already saved for this image, so revisiting it shows what's
    there instead of a blank canvas. `kind=pool` reads the bank's labels,
    `kind=test` reads the test set's ground truth -- both use the labels/ +
    classes.txt layout."""
    inp = checked_path(input_dir)
    d = deps.test_dir(inp) if kind == "test" else deps.state_dir(inp)
    img = checked_path(image)
    return {"boxes": groundtruth.read_boxes(str(d), str(img))}


@router.post("/label")
def save_label(req: dict, user: str | None = Depends(current_user)):
    """Extract embeddings for `boxes` and add them to the prompt bank (one
    embedding per distinct class in `boxes`, averaged over however many boxes
    of that class this image has), then write the YOLO label file. `409` if
    `model_id` doesn't match a model this bank is already locked to -- see
    `POST /api/reembed` to change it deliberately instead. `400` if `image`
    is flagged in the test set -- teaching the bank from a held-out image
    would make /api/evaluate measure memorization instead of generalization."""
    inp = checked_path(req["input_dir"])
    img = cv2.imread(str(checked_path(req["image"])))
    if img is None:
        raise HTTPException(400, "cannot read image")
    boxes = req["boxes"]
    if not boxes:
        raise HTTPException(400, "no boxes")
    if groundtruth.is_test(str(deps.test_dir(inp)), req["image"]):
        raise HTTPException(400, "this image is in the test set -- it can never be taught to the model")
    bank = Bank(str(deps.state_dir(inp)))
    try:
        model_id = bank.lock_model(req.get("model_id", model_registry.DEFAULT_MODEL_ID))
    except ValueError as exc:
        raise HTTPException(409, str(exc))

    by_class: dict[str, list[list[float]]] = {}
    for b in boxes:
        by_class.setdefault(b["cls"], []).append(b["box"])
    for cls_name, cls_boxes in by_class.items():
        bank.add(cls_name, extract_embedding(img, cls_boxes, model_id), req["image"], cls_boxes[0], user)
    bank.mark_labeled(req["image"])

    h, w = img.shape[:2]
    bank.write_yolo_labels(req["image"], boxes, w, h, merge=(req.get("mode", "replace") == "update"))
    return {"bank": bank.summary()}


@router.post("/predict")
def predict(req: dict):
    """FR-19 / T-05 — the model's guesses for ONE image, so the user corrects
    instead of drawing from scratch. Empty bank -> empty list, no forward pass.

    ponytail: synchronous, and arm() mutates the process-wide model, same as
    every other inference path here. Fine for one image and one labeler; give
    predict its own model instance if a second concurrent user shows up."""
    bank = Bank(str(deps.state_dir(checked_path(req["input_dir"]))))
    mean = bank.mean_vpe()
    if mean is None:
        return {"boxes": []}
    names, combined = mean
    model = arm(names, combined, bank.model_or_default)
    dets = predict_one(model, names, str(checked_path(req["image"])),
                        req.get("conf", 0.25), req.get("conf_by_class", {}))
    return {"boxes": dets}


@router.get("/history")
def get_history(input_dir: str):
    """T-07 — every Evaluate run this project has recorded, for the accuracy-
    over-time chart. Lives on disk next to the bank, so it survives a reload."""
    return {"history": bank_store.read_history(str(deps.state_dir(checked_path(input_dir))))}


@router.post("/history")
def add_history(req: dict):
    """Append one point (read-modify-write, no lock -- see bank.py's
    append_history) and return the history as it now stands."""
    d = deps.state_dir(checked_path(req["input_dir"]))
    return {"history": bank_store.append_history(str(d), req["point"])}


@router.delete("/history")
def del_history(input_dir: str):
    d = deps.state_dir(checked_path(input_dir))
    bank_store.history_path(str(d)).unlink(missing_ok=True)
    return {"history": []}


@router.post("/events")
def add_event(req: dict, user: str | None = Depends(current_user)):
    """§7 — record one thing that happened, so "does this tool save time" has
    an answer that outlives the tab. Fire-and-forget: the UI never waits on it
    and never shows an error for it."""
    d = deps.state_dir(checked_path(req["input_dir"]))
    event_log.append(str(d), {k: v for k, v in req.items() if k != "input_dir"} | {"user": user})
    return {"ok": True}


@router.get("/events")
def get_events(input_dir: str):
    d = deps.state_dir(checked_path(input_dir))
    return {"summary": event_log.summary(str(d))}


@router.post("/relabel")
def relabel(req: dict):
    """Rewrite this image's YOLO label file directly -- no embedding
    extraction, no bank.add, no mark_labeled. For fixing generated labels
    (delete an over-prediction, drag in a box the model missed) without the
    correction being treated as a new visual prompt. `boxes` may be empty --
    that's a legitimate "the model was wrong about everything here"."""
    inp = checked_path(req["input_dir"])
    if groundtruth.is_test(str(deps.test_dir(inp)), req["image"]):
        raise HTTPException(400, "this image is in the test set -- it can never be taught to the model")
    bank = Bank(str(deps.state_dir(inp)))
    boxes = req["boxes"]
    unknown = {b["cls"] for b in boxes} - set(bank.classes)
    if unknown:
        raise HTTPException(
            400, f"unknown class(es) {sorted(unknown)} -- use Save to bank to teach a new class"
        )
    img = cv2.imread(str(checked_path(req["image"])))
    if img is None:
        raise HTTPException(400, "cannot read image")
    h, w = img.shape[:2]
    bank.write_yolo_labels(req["image"], boxes, w, h, merge=(req.get("mode", "replace") == "update"))
    return {"bank": bank.summary()}
