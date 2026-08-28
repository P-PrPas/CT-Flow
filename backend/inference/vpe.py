"""YOLOE visual-prompt embedding extraction + inference with a prompt bank.

Mirrors the get_vpe() -> set_classes() pattern already proven in
poc/vp_studio/inference.py, just against single images instead of video frames.
"""
import threading
from contextlib import contextmanager

import cv2
import torch
from ultralytics import YOLOE
from ultralytics.models.yolo.yoloe import YOLOEVPSegPredictor

from . import models as model_registry

# ponytail: every model_id ever used stays resident for the life of the
# process -- fine for one or two checkpoints, but switching between many
# large ones in one long-running server will grow VRAM without bound. Add
# an LRU with model.to("cpu") eviction if that turns from hypothetical into
# a real OOM.
_models: dict[str, YOLOE] = {}
_predictors: dict[str, YOLOEVPSegPredictor] = {}

# arm() and set_prompts() both write to the shared YOLOE object -- class names,
# nc, the SAVPE vectors -- and predict()/get_vpe() read it straight back. Two
# projects using the same checkpoint therefore cannot be in flight at once: the
# second arm() overwrites the first one's classes mid-batch and its predictions
# come back decoded against the wrong class list. No exception, no wrong shape,
# just wrong answers.
#
# Today that is hidden rather than fixed: sync FastAPI endpoints and
# BackgroundTasks happen to serialise this work. The Go API will call this
# service genuinely in parallel, so the serialisation has to become explicit
# before it does.
#
# RLock, not Lock: a caller holding a batch's lock (see armed()) may reach
# extract_embedding() on the same thread, which takes it again.
# ponytail: one lock per checkpoint, held across a whole batch. It costs
# nothing today -- one GPU cannot run two passes at once anyway. Give each
# project its own model instance if that stops being true.
_arm_locks: dict[str, threading.RLock] = {}
_arm_locks_guard = threading.Lock()


def model_lock(model_id: str) -> threading.RLock:
    with _arm_locks_guard:
        return _arm_locks.setdefault(model_id, threading.RLock())


def get_model(model_id: str = model_registry.DEFAULT_MODEL_ID) -> YOLOE:
    if model_id not in _models:
        _models[model_id] = YOLOE(model_registry.checkpoint_path(model_id))
    return _models[model_id]


def _get_predictor(model_id: str, model: YOLOE) -> YOLOEVPSegPredictor:
    if model_id not in _predictors:
        predictor = YOLOEVPSegPredictor(
            overrides={
                "task": model.model.task,
                "mode": "predict",
                "save": False,
                "verbose": False,
                "batch": 1,
                "device": None,  # ultralytics auto-picks cuda:0 when available
                "imgsz": model.overrides.get("imgsz", 640),
            },
            _callbacks=model.callbacks,
        )
        predictor.setup_model(model=model.model)
        _predictors[model_id] = predictor
    return _predictors[model_id]


def extract_embedding(image_bgr, boxes: list[list[float]],
                       model_id: str = model_registry.DEFAULT_MODEL_ID) -> torch.Tensor:
    """boxes: list of [x1, y1, x2, y2] in pixel coords, all treated as the
    same class -> one averaged embedding for this labeling action.

    set_prompts() writes to the shared predictor and get_vpe() reads it back,
    so the pair has to be atomic per checkpoint -- see model_lock()."""
    model = get_model(model_id)
    predictor = _get_predictor(model_id, model)
    with model_lock(model_id):
        predictor.set_prompts(dict(bboxes=[list(b) for b in boxes], cls=[0] * len(boxes)))
        return predictor.get_vpe(image_bgr)


def arm(names: list[str], combined_vpe: torch.Tensor,
        model_id: str = model_registry.DEFAULT_MODEL_ID) -> YOLOE:
    """Set the prompt bank's classes on the model. Callers doing many images
    call this once and reuse the returned model, so class-setting cost is
    paid once per batch rather than once per image.

    model_id must be the same checkpoint the bank's embeddings were extracted
    with -- Bank.lock_model() is what enforces that upstream, this function
    just trusts its caller."""
    model = get_model(model_id)
    model.model.model[-1].nc = len(names)
    model.model.names = {i: n for i, n in enumerate(names)}
    model.model.set_classes(names, combined_vpe)
    # The predictor's AutoBackend keeps its own copy of `names`, taken when it
    # was first built. NMS splits the prediction tensor at 4 + len(those names)
    # -- so once the bank gains a class, a stale copy makes it slice the class
    # and mask columns apart in the wrong place and the mask head dies with a
    # shape error. Only shows up on an image where something is actually
    # detected, which is why it hid: teach one class, rescore, teach a second,
    # rescore -> 500.
    if getattr(model, "predictor", None) is not None:
        model.predictor.model.names = model.model.names
    return model


@contextmanager
def armed(names: list[str], combined_vpe: torch.Tensor,
          model_id: str = model_registry.DEFAULT_MODEL_ID):
    """arm() plus exclusive use of the checkpoint for as long as the caller
    needs it:

        with armed(names, combined, model_id) as model:
            for path in paths:
                predict_one(model, names, path, conf)

    The lock has to span the whole loop, not each predict: arm() sets the class
    list once and every prediction after it is decoded against that list, so
    releasing between images lets another project re-arm halfway through and
    silently relabel the rest of the batch. Callers that arm and predict must
    use this rather than calling arm() directly."""
    with model_lock(model_id):
        yield arm(names, combined_vpe, model_id)


def predict_one(model: YOLOE, names: list[str], image_path: str,
                 conf: float = 0.25,
                 conf_by_class: dict[str, float] | None = None) -> list[dict]:
    """[{"cls": name, "box": [x1,y1,x2,y2], "conf": float}, ...] for one image
    against an already-armed model.

    conf_by_class overrides `conf` for the classes it names (FR-33). Classes in
    one dataset do not share a good threshold: on conveyor_pvc, `defect` peaks
    at 0.05 and scores exactly zero at 0.25, while `good_part` wants 0.25 --
    see docs/history/EXPERIMENT_T01_CONF.md. One global number has to sacrifice one of
    them.

    Runs inference at the lowest threshold in play and filters afterwards.
    NMS only ever suppresses a box with a *higher*-scoring one, so this returns
    what a per-class run would -- until the extra low-confidence boxes hit
    ultralytics' max_det cap (300). Stay above ~0.02 on a busy image."""
    img = cv2.imread(image_path)
    if img is None:
        return []
    # Clamped away from zero: conf=0.0 makes ultralytics' mask head throw a
    # shape error rather than return everything, and these numbers arrive from
    # a browser. 0.001 is "no threshold" for every practical purpose.
    floor = max(min([conf, *conf_by_class.values()]) if conf_by_class else conf, 0.001)
    pred = model.predict(img, conf=floor, verbose=False)[0]
    dets = []
    if pred.boxes is not None:
        for box, c, k in zip(pred.boxes.xyxy.tolist(),
                             pred.boxes.conf.tolist(),
                             pred.boxes.cls.tolist()):
            name = names[int(k)]
            if c >= (conf_by_class or {}).get(name, conf):
                dets.append({"cls": name, "box": box, "conf": c})
    return dets
