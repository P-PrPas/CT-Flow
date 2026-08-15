"""Drag-and-drop image upload (FR-29 / T-13), so someone who knows what a
defect looks like can bring their own images without a server path and an
engineer to put files there for them.

Three checks stand between the browser and the disk, in this order:
  1. the destination goes through checked_path() like every other path;
  2. the filename is reduced to a bare name -- no directory part survives;
  3. the bytes have to decode as an image. Extension is a fast reject, the
     decode is the one that actually decides.
"""
import os

import cv2
import numpy as np
from fastapi import APIRouter, File, Form, HTTPException, UploadFile

from .. import config
from ..deps import checked_path
from ..services import auth
from ..services.images import list_images

router = APIRouter(prefix="/api", tags=["upload"])

MAX_MB = float(os.getenv("LABEL_TOOL_MAX_UPLOAD_MB", "25"))
CHUNK = 1 << 20


async def _read_capped(f: UploadFile, limit: int) -> bytes | None:
    """The whole file, or None if it is over `limit`. Reads in chunks and
    stops at the cap, so an oversized upload costs the cap, not the file."""
    out = b""
    while len(out) <= limit:
        chunk = await f.read(CHUNK)
        if not chunk:
            return out
        out += chunk
    return None


@router.post("/upload")
async def upload(dir: str = Form(...), files: list[UploadFile] = File(...)):
    """Save images into `dir`. Existing names are skipped rather than
    overwritten -- an upload must never be able to replace an image someone
    has already labeled."""
    if config.MODE == "vm" and not auth.enabled():
        # T-13's precondition, enforced in code rather than left in a doc:
        # a shared deployment that accepts files from anyone who knows the URL
        # is a worse problem than not having upload at all.
        raise HTTPException(403, "set LABEL_TOOL_USERS before enabling upload on a shared server")

    d = checked_path(dir)
    d.mkdir(parents=True, exist_ok=True)
    limit = int(MAX_MB * 1024 * 1024)
    saved, skipped = [], []

    for f in files:
        name = os.path.basename(f.filename or "").strip()
        if not name or name.startswith(".") or "/" in name or "\\" in name:
            skipped.append({"name": f.filename or "", "reason": "bad filename"})
            continue
        if os.path.splitext(name)[1].lower() not in config.IMAGE_EXTS:
            skipped.append({"name": name, "reason": "not an image file type"})
            continue
        dst = d / name
        if dst.exists():
            skipped.append({"name": name, "reason": "already in this folder"})
            continue
        data = await _read_capped(f, limit)
        if data is None:
            skipped.append({"name": name, "reason": f"larger than {MAX_MB:g} MB"})
            continue
        if cv2.imdecode(np.frombuffer(data, np.uint8), cv2.IMREAD_COLOR) is None:
            skipped.append({"name": name, "reason": "not a readable image"})
            continue
        dst.write_bytes(data)
        saved.append(str(dst))

    return {"saved": saved, "skipped": skipped, "images": list_images(str(d))}
