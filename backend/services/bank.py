"""Prompt bank, stored inside the output directory. No workspace concept --
the output dir IS the project:

    <output_dir>/
        _bank/embeddings.pt    per-class list of instance embeddings
        _bank/metadata.json    per-instance provenance + model lock

Labels, classes.txt and the labeled/auto status that used to live here moved
to PostgreSQL (T-21/T-22) -- see services/annotations_db.py and
docs/DB_MIGRATION_PLAN.md. Only the prompt bank itself -- embeddings and
their provenance -- is still a file; there's no pain point DB storage would
fix there, and torch tensors don't belong in a relational column.

Per-instance embeddings are kept (not just a running mean) so a future
nearest-neighbour matcher can use the same bank without relabeling.
"""
import json
import time
from pathlib import Path

import torch
from filelock import FileLock

from . import annotations_db
from . import models as model_registry

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
        # output_dir is always <input_dir>/.ctflow (see deps.state_dir) --
        # only used by summary()'s DB-backed labeled/auto lookup, so a bank
        # built against some other directory (as a couple of smoke-test
        # fixtures do, to exercise embeddings in isolation) just never calls
        # summary().
        self.input_dir = self.dir.parent
        self.bank_dir = self.dir / "_bank"
        self.bank_dir.mkdir(parents=True, exist_ok=True)
        self.emb_path = self.bank_dir / "embeddings.pt"
        self.meta_path = self.bank_dir / "metadata.json"
        self.lock = FileLock(str(self.bank_dir / ".lock"))
        self._load()
        self._migrate_missing_model()

    def _migrate_missing_model(self):
        """Backfill `model` for a bank written before FR-36 (model selection):
        it already has embeddings but metadata.json predates the `model` key
        entirely, so `self.model` reads as None even though the bank is very
        much locked in practice -- those embeddings only ever came from
        DEFAULT_MODEL_ID, the one checkpoint that existed back then. Without
        this, `bank.summary()["model"]` stays null forever, the frontend keeps
        rendering an editable model picker for an already-taught project, and
        picking a different model there does nothing (evaluate/predict never
        read it) or -- worse -- silently locks the bank to a model that
        doesn't match its actual embeddings the next time a label is saved,
        since lock_model() only rejects a *mismatch*, not a *None*.

        Runs on every construction of an old bank until it succeeds, so it's
        wrapped defensively: a read-only request (open a session, predict)
        must not 500 just because this particular write failed (seen for
        real: an output dir whose _bank/ predates the container's non-root
        user ends up root-owned and unwritable by `app` -- fix the
        underlying permissions, don't let a nice-to-have migration take
        unrelated reads down while that's unfixed). `model_or_default`
        remains the safety net for inference either way."""
        if self.model is not None or not self.classes:
            return
        try:
            with self.lock:
                self._load()
                if self.model is None and self.classes:
                    self.model = model_registry.DEFAULT_MODEL_ID
                    self._save()
        except OSError:
            pass

    def _load(self):
        self.embeddings: dict[str, list[torch.Tensor]] = (
            torch.load(self.emb_path, weights_only=False) if self.emb_path.exists() else {}
        )
        meta = (
            json.loads(self.meta_path.read_text(encoding="utf-8"))
            if self.meta_path.exists() else {}
        )
        self.instances: dict[str, list[dict]] = meta.get("instances", {})
        # None = no embedding has ever landed in this bank yet, so any model
        # is still fair game. Once set it never changes -- see lock_model().
        self.model: str | None = meta.get("model")

    def _save(self):
        torch.save(self.embeddings, self.emb_path)
        self.meta_path.write_text(
            json.dumps({"instances": self.instances, "model": self.model},
                       indent=2, ensure_ascii=False),
            encoding="utf-8",
        )

    def lock_model(self, model_id: str) -> str:
        """Every embedding in a bank has to come out of the same model's
        SAVPE head -- two checkpoints don't share an embedding space, so
        mixing them would feed set_classes() vectors that don't mean the same
        thing. The first embedding this bank ever receives decides the model
        for good, the same way Bank.classes locks insertion order. Returns
        the model now in effect (== model_id on a fresh bank)."""
        with self.lock:
            self._load()
            if self.model is None:
                self.model = model_id
                self._save()
            elif self.model != model_id:
                raise ValueError(
                    f"this project was taught with {self.model!r}, not {model_id!r} -- "
                    "start a new output folder to use a different model"
                )
            return self.model

    def reembed(self, model_id: str, new_embeddings: dict[str, list[torch.Tensor]]):
        """Swap every embedding in this bank for ones computed under a
        different checkpoint, and re-point the lock at it -- the only
        sanctioned way to change a project's model after its first label
        (see lock_model()). The caller (routers/jobs.py) does the actual
        re-extraction against every stored instance's (source_image, bbox);
        this only commits the result, atomically under the bank's lock, so a
        concurrent read never sees a half-swapped bank (old model id paired
        with new vectors, or vice versa).

        new_embeddings must cover exactly the bank's current classes with the
        same per-class instance count as what's on disk right now -- the
        caller reprocessed every existing instance, not some subset, and
        nothing here silently drops or invents one. Label files, instances
        (provenance), and class order are untouched; only the vectors and
        `model` change."""
        with self.lock:
            self._load()
            if set(new_embeddings) != set(self.classes):
                raise ValueError(
                    f"reembed must cover exactly {self.classes!r}, got {sorted(new_embeddings)!r}"
                )
            for name in self.classes:
                want, got = len(self.embeddings[name]), len(new_embeddings[name])
                if want != got:
                    raise ValueError(f"reembed count mismatch for {name!r}: had {want}, got {got}")
            # Rebuild through self.classes (not new_embeddings' own key order)
            # so insertion order -- what a label file's class index means --
            # survives even if the caller built its dict differently.
            self.embeddings = {name: new_embeddings[name] for name in self.classes}
            self.model = model_id
            self._save()

    @property
    def model_or_default(self) -> str:
        """`model` for anything that's about to run inference (predict/score/
        evaluate/autolabel). A bank that predates FR-36 (model selection) has
        embeddings but no recorded `model` -- there was only ever one
        checkpoint back then, so DEFAULT_MODEL_ID is what actually extracted
        them. Passing `None` straight to arm()/get_model() instead raises
        ValueError("unknown model None") the moment any of those run, which is
        exactly what happened to every pre-FR-36 project until this existed."""
        return self.model or model_registry.DEFAULT_MODEL_ID

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
        status = annotations_db.list_by_status(str(self.input_dir), "pool")
        return {
            "classes": [{"name": n, "count": self.count(n)} for n in self.classes],
            "labeled": status["labeled"],
            "auto": status["auto"],
            "model": self.model,  # null until the first box is saved -- model picker stays open until then
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

    def mean_vpe(self) -> tuple[list[str], torch.Tensor] | None:
        """One averaged embedding per class -- the prototype fed to
        YOLOE.set_classes(). ponytail: mean pooling, swap for NN matching over
        self.embeddings when a class turns out to be multi-modal."""
        names = self.classes
        if not names:
            return None
        per_class = [torch.mean(torch.stack(self.embeddings[n]), dim=0) for n in names]
        return names, torch.cat(per_class, dim=1)
