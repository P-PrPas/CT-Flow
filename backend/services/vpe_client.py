"""HTTP client for the inference sidecar (backend/vpe_service.py).

Everything the routers used to reach by importing services/vpe.py and
services/bank.py now goes through here. Deliberately thin: it is the seam the
Go port replaces first (docs/REFACTOR_PLAN.md phase 2), so the less judgement
it carries the less there is to reimplement -- and the shape of this file is
the specification for internal/vpe in Go.

No FastAPI import, per this backend's split between routers and services: a
sidecar error surfaces as VpeError carrying the status and detail it came with,
and app.py turns that back into the same {"detail": ...} body the caller would
have got when this code ran in-process.
"""
import json
import os

import httpx

VPE_URL = os.getenv("VPE_URL", "http://127.0.0.1:8001")

# Long: a request here can cover a full inference pass over a folder. The
# sidecar streams progress, so a stalled connection still gets noticed by the
# absence of lines rather than by this timeout.
TIMEOUT = httpx.Timeout(connect=10.0, read=600.0, write=60.0, pool=10.0)

_client = httpx.Client(base_url=VPE_URL, timeout=TIMEOUT)


class VpeError(Exception):
    """A failure the sidecar reported, with the status it chose. Passed
    through unchanged (409 model mismatch, 400 empty bank, 403 bad path) --
    the frontend matches on these exact messages."""

    def __init__(self, status: int, detail: str):
        super().__init__(detail)
        self.status = status
        self.detail = detail


def _raise_for(r: httpx.Response):
    if r.status_code >= 400:
        try:
            detail = r.json().get("detail", r.text)
        except ValueError:
            detail = r.text
        raise VpeError(r.status_code, detail)


def _post(path: str, payload: dict) -> dict:
    r = _client.post(path, json=payload)
    _raise_for(r)
    return r.json()


def bank(state_dir: str) -> dict:
    r = _client.get("/vpe/bank", params={"state_dir": state_dir})
    _raise_for(r)
    return r.json()


def total_instances(state_dir: str) -> dict:
    r = _client.get("/vpe/total_instances", params={"state_dir": state_dir})
    _raise_for(r)
    return r.json()


def teach(state_dir: str, image: str, boxes: list[dict], model_id: str | None,
          labeled_by: str | None) -> dict:
    return _post("/vpe/teach", {"state_dir": state_dir, "image": image, "boxes": boxes,
                                "model_id": model_id, "labeled_by": labeled_by})


def predict(state_dir: str, image: str, conf: float, conf_by_class: dict) -> dict:
    return _post("/vpe/predict", {"state_dir": state_dir, "image": image,
                                  "conf": conf, "conf_by_class": conf_by_class})


def _stream(path: str, payload: dict):
    """Yield one dict per NDJSON line. A line carrying "error" is the sidecar
    failing after its headers went out -- there is no status code left to
    change, so it has to be raised from here or the caller would read a
    truncated pass as a complete one."""
    with _client.stream("POST", path, json=payload) as r:
        if r.status_code >= 400:
            r.read()
            _raise_for(r)
        for line in r.iter_lines():
            if not line:
                continue
            item = json.loads(line)
            if "error" in item:
                raise VpeError(500, item["error"])
            yield item


def predict_stream(state_dir: str, images: list[str], conf: float,
                   conf_by_class: dict, want_sig: bool = False):
    """One {"image", "boxes", "sig"?} per image, then {"done": True}. The
    sidecar arms the checkpoint once for the whole pass and holds it, so the
    caller must consume this to the end rather than abandoning it part way."""
    return _stream("/vpe/predict_stream",
                   {"state_dir": state_dir, "images": images, "conf": conf,
                    "conf_by_class": conf_by_class, "want_sig": want_sig})


def reembed_stream(state_dir: str, model_id: str):
    """{"done_count": n} per instance, then {"done": True, classes, model}."""
    return _stream("/vpe/reembed_stream", {"state_dir": state_dir, "model_id": model_id})
