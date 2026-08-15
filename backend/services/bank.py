"""Prompt bank, stored inside the output directory. No workspace concept --
the output dir IS the project:

    <output_dir>/
        _bank/embeddings.pt    per-class list of instance embeddings
        _bank/metadata.json    per-instance provenance + which images are done
        labels/<stem>.txt      YOLO-format labels (the actual deliverable)
        classes.txt            class index -> name, matches the label files

Per-instance embeddings are kept (not just a running mean) so a future
nearest-neighbour matcher can use the same bank without relabeling.
"""
import json
import time
from pathlib import Path

import torch
from filelock import FileLock

from . import yolo_labels

HISTORY_MAX = 200


def history_path(output_dir: str) -> Path:
    return Path(output_dir) / "_bank" / "eval_history.json"


def read_history(output_dir: str) -> list[dict]:
    """T-07: every Evaluate run, kept next to the bank it measured. Lives on
    disk so the learning curve survives a browser, a machine, and a colleague."""
    p = history_path(output_dir)
    if not p.exists():
        return []
    try:
        return json.loads(p.read_text(encoding="utf-8"))
    except json.JSONDecodeError:
        return []  # a truncated history is a nicety to lose, never an error to raise


def append_history(output_dir: str, point: dict) -> list[dict]:
    # ponytail: read-modify-write, no lock. Two people evaluating the same
    # output_dir in the same second would drop a point -- reuse Bank.lock if
    # that ever stops being hypothetical.
    p = history_path(output_dir)
    p.parent.mkdir(parents=True, exist_ok=True)
    hist = (read_history(output_dir) + [point])[-HISTORY_MAX:]
    p.write_text(json.dumps(hist, ensure_ascii=False), encoding="utf-8")
    return hist


class Bank:
    def __init__(self, output_dir: str):
        self.dir = Path(output_dir)
        self.bank_dir = self.dir / "_bank"
        self.bank_dir.mkdir(parents=True, exist_ok=True)
        (self.dir / "labels").mkdir(exist_ok=True)
        self.emb_path = self.bank_dir / "embeddings.pt"
        self.meta_path = self.bank_dir / "metadata.json"
        self.lock = FileLock(str(self.bank_dir / ".lock"))
        self._load()

    def _load(self):
        self.embeddings: dict[str, list[torch.Tensor]] = (
            torch.load(self.emb_path, weights_only=False) if self.emb_path.exists() else {}
        )
        meta = (
            json.loads(self.meta_path.read_text(encoding="utf-8"))
            if self.meta_path.exists() else {}
        )
        self.instances: dict[str, list[dict]] = meta.get("instances", {})
        self.labeled: list[str] = meta.get("labeled", [])
        self.auto: list[str] = meta.get("auto", [])  # written by the model, not a human

    def _save(self):
        torch.save(self.embeddings, self.emb_path)
        self.meta_path.write_text(
            json.dumps({"instances": self.instances, "labeled": self.labeled,
                        "auto": self.auto},
                       indent=2, ensure_ascii=False),
            encoding="utf-8",
        )
        (self.dir / "classes.txt").write_text("\n".join(self.classes), encoding="utf-8")

    @property
    def classes(self) -> list[str]:
        # Insertion order, not sorted() -- a label file's class column is an
        # index into this list, so it must never shift once assigned. dict
        # order is insertion order in Python and survives torch.save/load
        # (plain pickle) as long as we never delete-and-reinsert a key.
        return list(self.embeddings.keys())

    def count(self, name: str) -> int:
        return len(self.embeddings.get(name, []))

    def summary(self) -> dict:
        return {
            "classes": [{"name": n, "count": self.count(n)} for n in self.classes],
            "labeled": self.labeled,
            "auto": self.auto,
        }

    def add(self, class_name: str, embedding: torch.Tensor, source_image: str,
            bbox: list[float], labeled_by: str | None = None):
        # re-read under the lock so a concurrent writer isn't clobbered
        with self.lock:
            self._load()
            self.embeddings.setdefault(class_name, []).append(embedding)
            # FR-31: labeled_by is None when auth is off. Recorded per instance
            # rather than per image because that is the grain a bad prompt has
            # to be traced back at -- one image can teach several.
            self.instances.setdefault(class_name, []).append(
                {"source_image": source_image, "bbox": bbox, "added_at": time.time(),
                 "labeled_by": labeled_by}
            )
            self._save()

    def mark_labeled(self, image_path: str):
        with self.lock:
            self._load()
            if image_path not in self.labeled:
                self.labeled.append(image_path)
            self._save()

    def mark_auto(self, image_paths: list[str]):
        with self.lock:
            self._load()
            for p in image_paths:
                if p not in self.auto and p not in self.labeled:
                    self.auto.append(p)
            self._save()

    def write_yolo_labels(self, image_path: str, boxes: list[dict], width: int, height: int,
                          merge: bool = False):
        """boxes: [{"cls": name, "box": [x1,y1,x2,y2]}] in pixel coords. By
        default this replaces the image's entire label file; merge=True reads
        what's already there first and writes the union, for adding a box to
        an already-labeled image without retyping the rest."""
        if merge:
            boxes = yolo_labels.read_boxes(str(self.dir), image_path, self.classes) + boxes
        yolo_labels.write_boxes(str(self.dir), image_path, boxes, width, height, self.classes)

    def mean_vpe(self) -> tuple[list[str], torch.Tensor] | None:
        """One averaged embedding per class -- the prototype fed to
        YOLOE.set_classes(). ponytail: mean pooling, swap for NN matching over
        self.embeddings when a class turns out to be multi-modal."""
        names = self.classes
        if not names:
            return None
        per_class = [torch.mean(torch.stack(self.embeddings[n]), dim=0) for n in names]
        return names, torch.cat(per_class, dim=1)
