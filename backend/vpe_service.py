"""The inference sidecar: everything that needs torch, and nothing else.

This is the half of the backend that stays Python (docs/REFACTOR_PLAN.md).
YOLOE's SAVPE head has no Go equivalent and will not get one, and the prompt
bank is a torch.save of per-class tensor lists, so the bank and the model
cannot live on opposite sides of the port -- Bank.lock_model()/reembed() commit
atomically under one FileLock, which only works with a single writer.

    uvicorn backend.vpe_service:app --port 8001

So: this process owns <input_dir>/.ctflow/_bank/ outright. Nothing else writes
there. In exchange it owns nothing else -- no database, no uploads, no auth, no
path picking -- and the API service assembles what the frontend actually sees
(BankSummary's labeled/auto come from PostgreSQL, which this process never
connects to).

The API service sends `state_dir` (the .ctflow directory), not `input_dir`, so
the ".ctflow" convention has exactly one owner and it is not this file.

**Not for direct exposure.** The caller has already run the path through its own
checked_path(); the roots check below is a second line of defence for a
misconfigured deployment, not the primary one. docker-compose.yml must not
publish this port.
"""
import json
import os
from pathlib import Path

import cv2
from fastapi import FastAPI, HTTPException
from fastapi.responses import StreamingResponse

from .services import models as model_registry
from .services.bank import Bank
from .services.vpe import armed, extract_embedding, model_lock, predict_one

app = FastAPI(title="CT-Flow VPE", version="1.0.0")

# Same two variables the API service reads, so a vm-mode deployment confines
# both processes to the same root. Duplicated rather than imported because
# backend/config.py belongs to the API service and goes away with it.
MODE = os.getenv("LABEL_TOOL_MODE", "local").lower()
VM_DATA_ROOT = Path(os.getenv("LABEL_TOOL_VM_ROOT", "/opt/mount/project"))


def checked(path: str) -> Path:
    p = Path(path)
    if MODE == "vm":
        try:
            p.resolve().relative_to(VM_DATA_ROOT.resolve())
        except ValueError:
            raise HTTPException(403, f"path outside {VM_DATA_ROOT} (vm mode)")
    return p


def _bank(state_dir: str) -> Bank:
    return Bank(str(checked(state_dir)))


def _ndjson(lines):
    """One JSON object per line. A stream can't change its status code after
    the first byte, so a failure mid-pass arrives as a line carrying "error"
    and the caller has to look for it -- see the API service's vpe client."""
    def gen():
        try:
            for line in lines:
                yield json.dumps(line, ensure_ascii=False) + "\n"
        except Exception as exc:
            yield json.dumps({"error": str(exc)}) + "\n"
    return StreamingResponse(gen(), media_type="application/x-ndjson")


@app.get("/vpe/health")
def health():
    """Container healthcheck. Deliberately does not touch a model: readiness
    here means "the process is up", and the first real request pays the load.
    A healthcheck that loaded a checkpoint would keep the service marked
    unhealthy for the minute that takes on a cold start."""
    return {"ok": True}


@app.get("/vpe/bank")
def get_bank(state_dir: str):
    """What this bank has been taught and which checkpoint taught it. The
    API service joins this with image status from PostgreSQL to build the
    BankSummary the frontend reads."""
    return _bank(state_dir).classes_summary()


@app.post("/vpe/teach")
def teach(req: dict):
    """Extract SAVPE embeddings for `boxes` and add them to the bank.

    Boxes are grouped by class here rather than by the caller: one embedding
    per class per save, averaged over that class's boxes in this image, is a
    property of what the bank stores, not of the HTTP layer. (A consequence
    worth knowing: reembed can only replay one bbox per instance -- see
    reembed_stream.)

    lock_model() runs before any inference, so a mismatched model_id costs a
    409 rather than a wasted checkpoint load."""
    bank = _bank(req["state_dir"])
    image_path = str(checked(req["image"]))
    img = cv2.imread(image_path)
    if img is None:
        raise HTTPException(400, "cannot read image")
    boxes = req["boxes"]
    if not boxes:
        raise HTTPException(400, "no boxes")
    try:
        model_id = bank.lock_model(req.get("model_id") or model_registry.DEFAULT_MODEL_ID)
    except ValueError as exc:
        raise HTTPException(409, str(exc))

    by_class: dict[str, list[list[float]]] = {}
    for b in boxes:
        by_class.setdefault(b["cls"], []).append(b["box"])
    for cls_name, cls_boxes in by_class.items():
        bank.add(cls_name, extract_embedding(img, cls_boxes, model_id),
                 req["image"], cls_boxes[0], req.get("labeled_by"))
    return bank.classes_summary()


@app.post("/vpe/predict")
def predict(req: dict):
    """FR-19 -- the model's guesses for one image. An empty bank costs nothing:
    no checkpoint load, no forward pass."""
    bank = _bank(req["state_dir"])
    mean = bank.mean_vpe()
    if mean is None:
        return {"boxes": []}
    names, combined = mean
    with armed(names, combined, bank.model_or_default) as model:
        dets = predict_one(model, names, str(checked(req["image"])),
                           req.get("conf", 0.25), req.get("conf_by_class") or {})
    return {"boxes": dets}


def _signature(path: str) -> list[int]:
    """FR-18 -- an 8x8 grayscale thumbnail, so the UI can tell "another frame of
    the same thing" from "something new" and stop offering five near-identical
    images in a row.

    Kept on this side of the port even though it needs no model: cv2's
    INTER_AREA and a Go resize do not produce the same 64 integers, and FR-18's
    ordering should not change just because the API was rewritten.

    ponytail: a perceptual thumbnail, not bank-embedding distance; it catches
    conveyor near-duplicates, which is the actual complaint. Swap in embeddings
    if it ever misses a case that matters."""
    img = cv2.imread(path, cv2.IMREAD_GRAYSCALE)
    if img is None:
        return []
    return cv2.resize(img, (8, 8), interpolation=cv2.INTER_AREA).flatten().tolist()


@app.post("/vpe/predict_stream")
def predict_stream(req: dict):
    """One line per image, for score/evaluate/autolabel. The caller drives its
    own progress bar off these lines and decides what to do with each result --
    which is why one endpoint serves all three passes.

    Streaming rather than a call per image because arm() must happen exactly
    once for the whole batch: it is the expensive part, and re-arming per image
    would also mean re-taking the checkpoint lock between images (see
    services/vpe.py::armed)."""
    bank = _bank(req["state_dir"])
    paths = [str(checked(p)) for p in req["images"]]
    conf = req.get("conf", 0.25)
    conf_by_class = req.get("conf_by_class") or {}
    want_sig = bool(req.get("want_sig"))
    mean = bank.mean_vpe()
    if mean is None:
        raise HTTPException(400, "prompt bank is empty -- label something first")
    names, combined = mean

    def lines():
        with armed(names, combined, bank.model_or_default) as model:
            for path in paths:
                out = {"image": path, "boxes": predict_one(model, names, path, conf, conf_by_class)}
                if want_sig:
                    out["sig"] = _signature(path)
                yield out
        yield {"done": True}

    return _ndjson(lines())


@app.post("/vpe/reembed_stream")
def reembed_stream(req: dict):
    """FR-39 -- re-extract every stored instance under a different checkpoint
    and swap the lock, the only sanctioned way to change a project's model
    after its first label.

    One line per instance for progress, then a final line with the new summary.
    The commit happens once, at the end, under the bank's lock: a failure part
    way through (a source image moved or deleted) leaves the old bank entirely
    untouched rather than half-swapped."""
    bank = _bank(req["state_dir"])
    model_id = req["model_id"]
    if bank.model is None:
        raise HTTPException(400, "this project has no model yet -- just label normally, no need to reembed")
    if model_id == bank.model:
        raise HTTPException(400, f"already using {model_id!r}")
    try:
        model_registry.checkpoint_path(model_id)  # fail fast on a bad id, before streaming
    except ValueError as exc:
        raise HTTPException(400, str(exc))

    def lines():
        new_embeddings: dict[str, list] = {}
        done = 0
        # Held across every instance: re-extraction is a batch like any other
        # pass, and releasing between instances would let another project's
        # teach interleave on the same checkpoint.
        with model_lock(model_id):
            for cls_name in bank.classes:
                vecs = []
                for inst in bank.instances[cls_name]:
                    img = cv2.imread(inst["source_image"])
                    if img is None:
                        raise ValueError(
                            f"cannot re-read {inst['source_image']!r} -- has it moved or been deleted?")
                    # ponytail: an instance taught from several same-class boxes
                    # in one save stores only the first box's coordinates (see
                    # teach), so reembed can only replay that one box and a
                    # multi-box instance loses the averaging it originally had.
                    # Rare in practice; store the full box list per instance if
                    # it turns out to matter.
                    vecs.append(extract_embedding(img, [inst["bbox"]], model_id))
                    done += 1
                    yield {"done_count": done}
                new_embeddings[cls_name] = vecs
        bank.reembed(model_id, new_embeddings)
        yield {"done": True, **bank.classes_summary()}

    return _ndjson(lines())


@app.get("/vpe/total_instances")
def total_instances(state_dir: str):
    """How many instances a reembed will process, so the caller can size its
    progress bar before starting the stream (`total` in its job response)."""
    bank = _bank(state_dir)
    return {"total": sum(len(i) for i in bank.instances.values()), "model": bank.model}
