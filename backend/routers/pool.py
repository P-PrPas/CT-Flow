"""The core labeling loop: open an image pool, save boxes into the prompt
bank, review/fix a generated label without touching the bank.

The prompt bank itself lives in the inference sidecar (backend/vpe_service.py)
-- this router assembles what the frontend sees out of two halves: what the
bank was taught (from the sidecar) and which images are labeled/auto (from
PostgreSQL, which the sidecar has no connection to)."""
import cv2
from fastapi import APIRouter, Depends, HTTPException
from fastapi.responses import FileResponse

from .. import deps
from ..deps import checked_path, current_user
from ..services import annotations_db
from ..services import events as event_log
from ..services import history as history_store
from ..services import vpe_client
from ..services.images import list_images

router = APIRouter(prefix="/api", tags=["pool"])


def bank_summary(input_dir: str, state_dir: str) -> dict:
    """BankSummary as docs/API_REFERENCE.md defines it. Assembled here rather
    than by either store, because neither one can see both halves."""
    status = annotations_db.list_by_status(input_dir, "pool")
    return vpe_client.bank(state_dir) | {"labeled": status["labeled"], "auto": status["auto"]}


@router.post("/session")
def open_session(req: dict):
    """Opens the one folder a project needs: the pool. The prompt bank lives
    in a fixed subfolder of it (see deps.state_dir); labels and the test-set
    manifest live in PostgreSQL, keyed by this same input_dir (see
    services/annotations_db.py) -- nothing else to browse for. Bundles the
    test-set state in the same response so the UI never has to make a second
    "did you forget the test set" round trip."""
    inp = checked_path(req["input_dir"])
    if not inp.is_dir():
        raise HTTPException(400, f"input dir not found: {inp}")
    images = list_images(str(inp))
    if not images:
        raise HTTPException(400, f"no images in {inp}")
    return {
        "images": images,
        "bank": bank_summary(str(inp), str(deps.state_dir(inp))),
        "testset": {
            "images": annotations_db.list_test_images(str(inp)),
            "labeled": sorted(annotations_db.labeled_stems(str(inp))),
            "classes": annotations_db.get_classes(str(inp), "testset"),
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
    `kind=test` reads the test set's ground truth."""
    inp = checked_path(input_dir)
    img = checked_path(image)
    db_kind = "testset" if kind == "test" else "pool"
    return {"boxes": annotations_db.read_boxes(str(inp), db_kind, str(img))}


@router.post("/label")
def save_label(req: dict, user: str | None = Depends(current_user)):
    """Extract embeddings for `boxes` and add them to the prompt bank (one
    embedding per distinct class in `boxes`, averaged over however many boxes
    of that class this image has), then write the label into PostgreSQL.
    `409` if `model_id` doesn't match a model this bank is already locked to
    -- see `POST /api/reembed` to change it deliberately instead. `400` if
    `image` is flagged in the test set -- teaching the bank from a held-out
    image would make /api/evaluate measure memorization instead of
    generalization.

    What is load-bearing about the order survives the split to the sidecar:
    the test-set check and the model lock both refuse before any inference
    runs, and the bank is taught before the database is written. Those two
    stores have never shared a transaction, so a failure between them still
    leaves an embedding with no annotation row -- same exposure as before,
    deliberately not widened here.

    One deliberate change: "cannot read image" used to be reported ahead of
    "no boxes" and the test-set refusal, because the router decoded the image
    itself before checking anything. The decode now happens in the sidecar, so
    a request that is *both* unreadable and empty (or unreadable and held out)
    reports the cheap reason instead. Each condition on its own still answers
    exactly as before; the alternative was decoding every image twice per save
    to preserve the precedence of two degenerate cases."""
    inp = checked_path(req["input_dir"])
    boxes = req["boxes"]
    if not boxes:
        raise HTTPException(400, "no boxes")
    if annotations_db.is_test(str(inp), req["image"]):
        raise HTTPException(400, "this image is in the test set -- it can never be taught to the model")
    state_dir = str(deps.state_dir(inp))
    vpe_client.teach(state_dir, str(checked_path(req["image"])), boxes,
                     req.get("model_id"), user)

    annotations_db.write_boxes(str(inp), "pool", req["image"], boxes, created_by=user,
                                merge=(req.get("mode", "replace") == "update"))
    annotations_db.mark_labeled(str(inp), "pool", req["image"])
    return {"bank": bank_summary(str(inp), state_dir)}


@router.post("/predict")
def predict(req: dict):
    """FR-19 / T-05 — the model's guesses for ONE image, so the user corrects
    instead of drawing from scratch. Empty bank -> empty list, no forward pass."""
    inp = checked_path(req["input_dir"])
    return vpe_client.predict(str(deps.state_dir(inp)), str(checked_path(req["image"])),
                              req.get("conf", 0.25), req.get("conf_by_class", {}))


@router.get("/history")
def get_history(input_dir: str):
    """T-07 — every Evaluate run this project has recorded, for the accuracy-
    over-time chart. Lives on disk next to the bank, so it survives a reload."""
    return {"history": history_store.read_history(str(deps.state_dir(checked_path(input_dir))))}


@router.post("/history")
def add_history(req: dict):
    """Append one point (read-modify-write, no lock -- see
    history.py's append_history) and return the history as it now stands."""
    d = deps.state_dir(checked_path(req["input_dir"]))
    return {"history": history_store.append_history(str(d), req["point"])}


@router.delete("/history")
def del_history(input_dir: str):
    d = deps.state_dir(checked_path(input_dir))
    history_store.history_path(str(d)).unlink(missing_ok=True)
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
def relabel(req: dict, user: str | None = Depends(current_user)):
    """Rewrite this image's label directly -- no embedding extraction, no
    bank.add, no mark_labeled. For fixing generated labels (delete an
    over-prediction, drag in a box the model missed) without the correction
    being treated as a new visual prompt. `boxes` may be empty -- that's a
    legitimate "the model was wrong about everything here"."""
    inp = checked_path(req["input_dir"])
    if annotations_db.is_test(str(inp), req["image"]):
        raise HTTPException(400, "this image is in the test set -- it can never be taught to the model")
    state_dir = str(deps.state_dir(inp))
    boxes = req["boxes"]
    taught = {c["name"] for c in vpe_client.bank(state_dir)["classes"]}
    unknown = {b["cls"] for b in boxes} - taught
    if unknown:
        raise HTTPException(
            400, f"unknown class(es) {sorted(unknown)} -- use Save to bank to teach a new class"
        )
    if cv2.imread(str(checked_path(req["image"]))) is None:
        raise HTTPException(400, "cannot read image")
    annotations_db.write_boxes(str(inp), "pool", req["image"], boxes, created_by=user,
                                merge=(req.get("mode", "replace") == "update"))
    return {"bank": bank_summary(str(inp), state_dir)}
