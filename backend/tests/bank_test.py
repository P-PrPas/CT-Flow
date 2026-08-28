"""Prompt-bank unit checks: the model lock, the pre-FR-36 self-heal, and
reembed's atomic commit.

    python -m backend.tests.bank_test

Split out of tests/smoke_test.py when the bank moved to the inference sidecar
(docs/history/REFACTOR_PLAN.md phase 1). These assertions construct Bank() directly and
therefore need torch, while everything left in the smoke test speaks HTTP -- and
the smoke test has to stay runnable without torch, because from phase 2 it
drives a Go binary from a machine that has no reason to have a CUDA wheel on it.

Nothing here loads a checkpoint. The vectors are hand-made tensors: what is
under test is the bookkeeping around embeddings, not the embeddings.
"""
import json
import os
import shutil
import tempfile
from pathlib import Path

import torch

from ..inference import models as model_registry
from ..inference.bank import Bank

# --- model lock, exercised directly (no HTTP, no model load needed) ---
LOCK = Path(os.getenv("BANK_TEST_DIR") or tempfile.mkdtemp(prefix="ctflow_bank_")) / "lock"
if LOCK.exists():
    shutil.rmtree(LOCK)
b = Bank(str(LOCK))
assert b.model is None
assert b.lock_model("yoloe-11s-seg") == "yoloe-11s-seg"
assert b.lock_model("yoloe-11s-seg") == "yoloe-11s-seg"  # same model again: no-op
try:
    b.lock_model("yoloe-11m-seg")
    assert False, "a second model on the same bank should have been rejected"
except ValueError:
    pass
assert Bank(str(LOCK)).model == "yoloe-11s-seg"  # persisted across a reload
shutil.rmtree(LOCK)
print("model lock: first embedding fixes the bank's model, survives reload, rejects mismatches")

# --- regression: a bank written before FR-36 has no "model" key at all (not
# just None) -- simulate that exact on-disk shape and confirm the bank heals
# itself the moment anything next constructs a Bank() for it, instead of
# staying permanently None (which used to crash predict/score/evaluate/
# autolabel with "unknown model None", and separately left the frontend
# showing an editable model picker for an already-taught project forever).
b = Bank(str(LOCK))
b.embeddings["ore"] = []  # give it a class so classes/mean_vpe aren't the reason it's empty
b._save()
(LOCK / "_bank" / "metadata.json").write_text(
    json.dumps({k: v for k, v in json.loads((LOCK / "_bank" / "metadata.json").read_text()).items() if k != "model"}),
    encoding="utf-8",
)
assert "model" not in json.loads((LOCK / "_bank" / "metadata.json").read_text())  # simulated shape confirmed

healed = Bank(str(LOCK))  # __init__ self-heals: has embeddings, no model key -> locks to the default
assert healed.model == model_registry.DEFAULT_MODEL_ID, healed.model
assert healed.model_or_default == model_registry.DEFAULT_MODEL_ID
assert json.loads((LOCK / "_bank" / "metadata.json").read_text())["model"] == model_registry.DEFAULT_MODEL_ID  # persisted, not just in-memory

try:
    healed.lock_model("yoloe-26x-seg")  # now genuinely locked -- a mismatch must reject, not silently switch
    assert False, "a healed bank should reject a different model like any other locked bank"
except ValueError:
    pass
shutil.rmtree(LOCK)
print("model lock: a pre-FR-36 bank (no model key on disk) self-heals to the default and locks for real")

# --- reembed: the only sanctioned way to change a bank's model after its
# first label. Exercised directly against Bank -- routers/jobs.py::_run_reembed
# is the thing that actually calls a model, this only tests the atomic commit.
b = Bank(str(LOCK))
b.add("a", torch.zeros(1, 4), "img1.jpg", [0, 0, 1, 1])
b.add("a", torch.ones(1, 4), "img2.jpg", [0, 0, 1, 1])
b.add("b", torch.full((1, 4), 2.0), "img3.jpg", [0, 0, 1, 1])
b.lock_model("yoloe-11s-seg")
instances_before = json.loads(json.dumps(b.instances))  # deep copy for comparison

new = {"a": [torch.full((1, 4), 9.0), torch.full((1, 4), 8.0)], "b": [torch.full((1, 4), 7.0)]}
b.reembed("yoloe-11m-seg", new)
assert b.model == "yoloe-11m-seg"
assert [t.tolist() for t in b.embeddings["a"]] == [[[9.0] * 4], [[8.0] * 4]]
assert [t.tolist() for t in b.embeddings["b"]] == [[[7.0] * 4]]
assert b.classes == ["a", "b"]  # insertion order survived the swap
assert b.instances == instances_before  # provenance untouched
assert Bank(str(LOCK)).model == "yoloe-11m-seg"  # persisted, not just in-memory

try:
    b.reembed("yoloe-11l-seg", {"a": new["a"]})  # missing class "b"
    assert False, "reembed covering fewer classes than the bank has should reject"
except ValueError:
    pass
try:
    b.reembed("yoloe-11l-seg", {"a": new["a"], "b": [torch.zeros(1, 4), torch.zeros(1, 4)]})  # wrong count for "b"
    assert False, "reembed with a mismatched instance count should reject"
except ValueError:
    pass
assert b.model == "yoloe-11m-seg"  # both rejected attempts left the bank exactly as it was
shutil.rmtree(LOCK)
print("reembed: swaps every embedding + the model lock atomically, rejects a partial/mismatched replacement")

print("BANK TEST OK")
