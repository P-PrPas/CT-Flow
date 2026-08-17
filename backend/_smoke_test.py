"""One runnable check: open a session, label a box, verify the bank + YOLO
labels land in the output dir and survive a reload, then rescore the pool.
Run from label_tool/: .venv\\Scripts\\python.exe -m backend._smoke_test"""
import json
import os
import shutil
import time
from pathlib import Path

from fastapi.testclient import TestClient

from .app import app
from .routers import uploads
from .services import auth
from .services import models as model_registry
from .services.bank import Bank

HERE = Path(__file__).parent
POOL = str(HERE / "test_pool")
OUT = HERE / "_smoke_out"
if OUT.exists():
    shutil.rmtree(OUT)

c = TestClient(app)


def wait_job(job_id: str, timeout: float = 60) -> dict:
    """Evaluate/autolabel/score all return a job_id + poll via /api/jobs/{id}
    for progress -- this is the same poll loop the UI runs."""
    start = time.time()
    while True:
        r = c.get(f"/api/jobs/{job_id}")
        assert r.status_code == 200, r.text
        j = r.json()
        if j["finished"]:
            assert j["error"] is None, j["error"]
            return j["result"]
        assert time.time() - start < timeout, f"job {job_id} did not finish in {timeout}s"
        time.sleep(0.1)

cfg = c.get("/api/config").json()
assert cfg["mode"] in ("local", "vm")
assert cfg["default_model"] == "yoloe-11s-seg", cfg
assert {"id", "family", "size", "available"} <= cfg["models"][0].keys() and len(cfg["models"]) > 1, cfg["models"]
assert all("file" not in m for m in cfg["models"]), cfg["models"]  # no local paths leak to the browser
default_entry = next(m for m in cfg["models"] if m["id"] == cfg["default_model"])
assert default_entry["available"] is True, default_entry  # CI/dev always has the default weight cached

r = c.post("/api/session", json={"input_dir": POOL, "output_dir": str(OUT)})
assert r.status_code == 200, r.text
images = r.json()["images"]
assert len(images) == 3, images
print("pool:", len(images), "images")

target = images[0]
r = c.post("/api/label", json={
    "output_dir": str(OUT),
    "image": target,
    "boxes": [{"cls": "test_item", "box": [30, 30, 120, 120]}],
})
assert r.status_code == 200, r.text
assert r.json()["bank"]["classes"] == [{"name": "test_item", "count": 1}]
# no model_id was sent -> the bank locks onto the default, not "unset"
assert r.json()["bank"]["model"] == "yoloe-11s-seg", r.json()["bank"]

# switching models on a bank that already has embeddings must be rejected --
# rejected before any inference runs, so this costs no extra model load
r = c.post("/api/label", json={
    "output_dir": str(OUT), "image": target,
    "boxes": [{"cls": "test_item", "box": [1, 1, 5, 5]}],
    "model_id": "yoloe-11m-seg",
})
assert r.status_code == 409, r.text
print("model lock: mismatched model_id on an existing bank rejected (409)")

label_file = OUT / "labels" / (Path(target).stem + ".txt")
assert label_file.exists() and label_file.read_text().startswith("0 "), label_file
print("yolo label:", label_file.read_text())

# reload from disk -- bank must round-trip
reloaded = Bank(str(OUT))
assert reloaded.classes == ["test_item"] and reloaded.labeled == [target]

r = c.post("/api/score", json={"output_dir": str(OUT), "images": images[1:]})
assert r.status_code == 200, r.text
scores = wait_job(r.json()["job_id"])["scores"]
assert set(scores) == set(images[1:]), scores
# FR-18: every score carries an 8x8 thumbnail the UI uses to spread picks
assert all(len(s["sig"]) == 64 for s in scores.values()), scores
print("scores:", {p: (s["conf"], s["cls"]) for p, s in scores.items()})

# a saved image should hand back its own boxes on request
r = c.get("/api/boxes", params={"dir": str(OUT), "image": target})
assert r.status_code == 200, r.text
saved = r.json()["boxes"]
assert saved and saved[0]["cls"] == "test_item", saved
print("saved boxes:", saved)

# mode="update": add a box without losing the one already there
r = c.post("/api/label", json={
    "output_dir": str(OUT), "image": target,
    "boxes": [{"cls": "test_item", "box": [200, 200, 260, 260]}],
    "mode": "update",
})
assert r.status_code == 200, r.text
r = c.get("/api/boxes", params={"dir": str(OUT), "image": target})
merged = r.json()["boxes"]
assert len(merged) == 2, merged  # original box survived, new one added
print("after update:", merged)

# mode="replace" (default) must still fully overwrite
r = c.post("/api/label", json={
    "output_dir": str(OUT), "image": target,
    "boxes": [{"cls": "test_item", "box": [5, 5, 15, 15]}],
})
assert r.status_code == 200, r.text
r = c.get("/api/boxes", params={"dir": str(OUT), "image": target})
replaced = r.json()["boxes"]
assert len(replaced) == 1, replaced
print("after replace:", replaced)

# class-order stability: a new class name that sorts before "test_item"
# must not shift the index test_item was already written under
r = c.post("/api/label", json={
    "output_dir": str(OUT), "image": images[1],
    "boxes": [{"cls": "aaa_new_class", "box": [1, 1, 20, 20]}],
})
assert r.status_code == 200, r.text
r = c.get("/api/boxes", params={"dir": str(OUT), "image": target})
still_correct = r.json()["boxes"]
assert still_correct and still_correct[0]["cls"] == "test_item", still_correct
print("class order stable after adding 'aaa_new_class':", still_correct)

# /api/relabel: fix a generated label directly -- no new embeddings, unlike /api/label
bank_before = Bank(str(OUT)).summary()
r = c.post("/api/relabel", json={
    "output_dir": str(OUT), "image": target,
    "boxes": [{"cls": "test_item", "box": [1, 1, 9, 9]}],
})
assert r.status_code == 200, r.text
assert r.json()["bank"]["classes"] == bank_before["classes"], r.json()["bank"]  # no embeddings added
r = c.get("/api/boxes", params={"dir": str(OUT), "image": target})
assert [b["cls"] for b in r.json()["boxes"]] == ["test_item"], r.json()

# deleting every box (boxes=[]) is a legitimate "model was wrong here"
r = c.post("/api/relabel", json={"output_dir": str(OUT), "image": target, "boxes": []})
assert r.status_code == 200, r.text
r = c.get("/api/boxes", params={"dir": str(OUT), "image": target})
assert r.json()["boxes"] == [], r.json()

# a class never taught to the bank must be rejected, not silently mis-indexed
r = c.post("/api/relabel", json={
    "output_dir": str(OUT), "image": target,
    "boxes": [{"cls": "never_seen_before", "box": [1, 1, 9, 9]}],
})
assert r.status_code == 400, r.text
print("relabel: bank untouched by review edits, empty boxes allowed, unknown class rejected")


# --- readiness loop: prepare a test set (ground truth only, no prompt bank) ---
TEST = HERE / "_smoke_test_set"
if TEST.exists():
    shutil.rmtree(TEST)
TEST.mkdir()

r = c.post("/api/testset/import", json={"test_dir": str(TEST), "images": [images[1]]})
assert r.status_code == 200, r.text
test_img = r.json()["images"][0]
assert r.json()["imported"] == [test_img]
assert Path(test_img).read_bytes() == Path(images[1]).read_bytes()  # copy, not a move
assert Path(images[1]).exists()  # pool untouched

# re-importing the same source must not clobber anything
r = c.post("/api/testset/import", json={"test_dir": str(TEST), "images": [images[1]]})
assert r.json()["imported"] == []

r = c.post("/api/testset/session", json={"test_dir": str(TEST)})
assert r.status_code == 200, r.text
assert r.json()["images"] == [test_img] and r.json()["labeled"] == []

r = c.post("/api/testset/label", json={
    "test_dir": str(TEST),
    "image": test_img,
    "boxes": [{"cls": "test_item", "box": [40, 40, 150, 150]}],
})
assert r.status_code == 200, r.text
assert r.json()["classes"] == ["test_item"]
print("ground truth:", r.json())

# mode="update" on ground truth too: add without losing what's there
r = c.post("/api/testset/label", json={
    "test_dir": str(TEST), "image": test_img,
    "boxes": [{"cls": "test_item", "box": [5, 5, 30, 30]}],
    "mode": "update",
})
assert r.status_code == 200, r.text
r = c.get("/api/boxes", params={"dir": str(TEST), "image": test_img})
gt_merged = r.json()["boxes"]
assert len(gt_merged) == 2, gt_merged
print("ground truth after update:", gt_merged)

# must not touch the prompt bank -- test images are held out, not prompts
assert not (TEST / "_bank").exists()

r = c.post("/api/testset/session", json={"test_dir": str(TEST)})
assert r.json()["labeled"] == [Path(test_img).stem]

# test-set manager: import a second image, then remove it -- file + label gone,
# the first image and the pool source untouched
r = c.post("/api/testset/import", json={"test_dir": str(TEST), "images": [images[2]]})
assert r.status_code == 200, r.text
second_img = [p for p in r.json()["imported"]][0]
r = c.post("/api/testset/label", json={
    "test_dir": str(TEST), "image": second_img,
    "boxes": [{"cls": "test_item", "box": [10, 10, 50, 50]}],
})
assert r.status_code == 200, r.text

r = c.post("/api/testset/remove", json={"test_dir": str(TEST), "images": [second_img]})
assert r.status_code == 200, r.text
assert r.json()["removed"] == [Path(second_img).stem]
assert r.json()["images"] == [test_img]
assert not Path(second_img).exists()
assert not (TEST / "labels" / f"{Path(second_img).stem}.txt").exists()
assert Path(images[2]).exists()  # pool source untouched
print("removed from test set:", r.json()["removed"])

r = c.post("/api/evaluate", json={"output_dir": str(OUT), "test_dir": str(TEST), "conf": 0.1})
assert r.status_code == 200, r.text
assert r.json()["total"] == 1
ev = wait_job(r.json()["job_id"])
assert ev["images"] == 1 and 0.0 <= ev["overall"]["f1"] <= 1.0, ev
img0 = ev["per_image"][0]
assert set(img0) >= {"image", "gt", "pred", "tp", "fp", "fn"}, img0
assert all("matched" in g for g in img0["gt"]), img0
print("eval:", ev["overall"], "| per-image gt/pred:", len(img0["gt"]), len(img0["pred"]))

r = c.post("/api/autolabel", json={"output_dir": str(OUT), "images": images[1:], "conf": 0.1})
assert r.status_code == 200, r.text
auto = wait_job(r.json()["job_id"])
assert auto["written"] + auto["no_detection"] == 2, auto
assert len(auto["bank"]["auto"]) == auto["written"]
# FR-28: the empty ones are named, not just counted
assert len(auto["no_detection_images"]) == auto["no_detection"], auto
print("autolabel:", auto["written"], "written,", auto["no_detection"], "empty")

# FR-19: pre-annotation for a single image, straight from the bank
r = c.post("/api/predict", json={"output_dir": str(OUT), "image": target, "conf": 0.05})
assert r.status_code == 200, r.text
drafts = r.json()["boxes"]
assert all(d["cls"] in Bank(str(OUT)).classes and len(d["box"]) == 4 for d in drafts), drafts
print("predict:", len(drafts), "draft box(es)")

# an empty bank must cost nothing rather than error
EMPTY_OUT = HERE / "_smoke_empty"
r = c.post("/api/predict", json={"output_dir": str(EMPTY_OUT), "image": target})
assert r.status_code == 200 and r.json()["boxes"] == [], r.text
shutil.rmtree(EMPTY_OUT)

# FR-09 / T-17: relabel mode="update" merges instead of replacing
c.post("/api/relabel", json={"output_dir": str(OUT), "image": target, "boxes": []})
for box in ([1, 1, 9, 9], [20, 20, 40, 40]):
    r = c.post("/api/relabel", json={
        "output_dir": str(OUT), "image": target,
        "boxes": [{"cls": "test_item", "box": box}], "mode": "update",
    })
    assert r.status_code == 200, r.text
merged_review = c.get("/api/boxes", params={"dir": str(OUT), "image": target}).json()["boxes"]
assert len(merged_review) == 2, merged_review
print("relabel update:", len(merged_review), "boxes kept")

# T-07: evaluate history round-trips through <output_dir>/_bank/eval_history.json
point = {"ts": 1, "conf": 0.1, "prompts": {"test_item": 3}, "totalPrompts": 3,
         "overall": ev["overall"], "perClass": ev["per_class"]}
assert c.post("/api/history", json={"output_dir": str(OUT), "point": point}).json()["history"] == [point]
assert c.get("/api/history", params={"output_dir": str(OUT)}).json()["history"] == [point]
assert (OUT / "_bank" / "eval_history.json").exists()
assert c.request("DELETE", "/api/history", params={"output_dir": str(OUT)}).json()["history"] == []
assert c.get("/api/history", params={"output_dir": str(OUT)}).json()["history"] == []
print("eval history: persisted, read back, cleared")


# Inference has to survive the bank gaining a class mid-process (it grew from
# 1 to 2 above). A threshold this low guarantees detections, which is what it
# takes to reach the mask head where a stale class count blows up -- see arm().
r = c.post("/api/predict", json={"output_dir": str(OUT), "image": target, "conf": 0.001})
assert r.status_code == 200 and r.json()["boxes"], r.text[:200]
assert {b["cls"] for b in r.json()["boxes"]} <= set(Bank(str(OUT)).classes), r.json()
print("predict after a class was added:", len(r.json()["boxes"]), "boxes, no shape error")

# FR-33: a per-class threshold above 1.0 can never be met, so naming every
# class in conf_by_class must empty the result even at conf=0.0
r = c.post("/api/predict", json={
    "output_dir": str(OUT), "image": target, "conf": 0.0,
    "conf_by_class": {n: 1.01 for n in Bank(str(OUT)).classes},
})
assert r.status_code == 200 and r.json()["boxes"] == [], r.text
r = c.post("/api/evaluate", json={
    "output_dir": str(OUT), "test_dir": str(TEST), "conf": 0.1,
    "conf_by_class": {"test_item": 0.9},
})
assert wait_job(r.json()["job_id"])["conf_by_class"] == {"test_item": 0.9}
print("conf_by_class: per-class thresholds honoured and echoed back")

# --- FR-31: every bank instance records who taught it (None when auth is off)
meta = json.loads((OUT / "_bank" / "metadata.json").read_text(encoding="utf-8"))
assert all("labeled_by" in i for insts in meta["instances"].values() for i in insts), meta

# --- model lock, exercised directly (no HTTP, no model load needed) ---
LOCK = HERE / "_smoke_lock"
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
import torch as _torch  # noqa: E402 -- local, smoke-test only

b = Bank(str(LOCK))
b.add("a", _torch.zeros(1, 4), "img1.jpg", [0, 0, 1, 1])
b.add("a", _torch.ones(1, 4), "img2.jpg", [0, 0, 1, 1])
b.add("b", _torch.full((1, 4), 2.0), "img3.jpg", [0, 0, 1, 1])
b.lock_model("yoloe-11s-seg")
labels_before, instances_before = b.labeled, json.loads(json.dumps(b.instances))  # deep copy for comparison

new = {"a": [_torch.full((1, 4), 9.0), _torch.full((1, 4), 8.0)], "b": [_torch.full((1, 4), 7.0)]}
b.reembed("yoloe-11m-seg", new)
assert b.model == "yoloe-11m-seg"
assert [t.tolist() for t in b.embeddings["a"]] == [[[9.0] * 4], [[8.0] * 4]]
assert [t.tolist() for t in b.embeddings["b"]] == [[[7.0] * 4]]
assert b.classes == ["a", "b"]  # insertion order survived the swap
assert b.labeled == labels_before and b.instances == instances_before  # provenance/labels untouched
assert Bank(str(LOCK)).model == "yoloe-11m-seg"  # persisted, not just in-memory

try:
    b.reembed("yoloe-11l-seg", {"a": new["a"]})  # missing class "b"
    assert False, "reembed covering fewer classes than the bank has should reject"
except ValueError:
    pass
try:
    b.reembed("yoloe-11l-seg", {"a": new["a"], "b": [_torch.zeros(1, 4), _torch.zeros(1, 4)]})  # wrong count for "b"
    assert False, "reembed with a mismatched instance count should reject"
except ValueError:
    pass
assert b.model == "yoloe-11m-seg"  # both rejected attempts left the bank exactly as it was
shutil.rmtree(LOCK)
print("reembed: swaps every embedding + the model lock atomically, rejects a partial/mismatched replacement")

# --- §7: effort metrics outlive the browser tab ---
for e in [{"kind": "session", "session": "s1"},
          {"kind": "label", "session": "s1", "secs": 12},
          {"kind": "auto", "session": "s1", "secs": 300, "written": 4},
          {"kind": "fix", "session": "s1"}]:
    assert c.post("/api/events", json={"output_dir": str(OUT)} | e).status_code == 200
stats = c.get("/api/events", params={"output_dir": str(OUT)}).json()["summary"]
assert stats["sessions"] == 1 and stats["abandonment"] == 0.0, stats
assert stats["median_label_secs"] == 12 and stats["correction_rate"] == 0.25, stats
assert (OUT / "_bank" / "events.jsonl").exists()
print("events:", stats)

# --- FR-29 / T-13: upload ---
UP = HERE / "_smoke_upload"
if UP.exists():
    shutil.rmtree(UP)
jpeg = Path(images[0]).read_bytes()

r = c.post("/api/upload", data={"dir": str(UP)},
           files=[("files", ("fresh.jpg", jpeg, "image/jpeg"))])
assert r.status_code == 200, r.text
assert r.json()["saved"] == [str(UP / "fresh.jpg")], r.json()

r = c.post("/api/upload", data={"dir": str(UP)}, files=[
    ("files", ("notes.txt", b"not an image", "text/plain")),
    ("files", ("fake.jpg", b"jpg in name only", "image/jpeg")),
    ("files", ("fresh.jpg", jpeg, "image/jpeg")),
    ("files", ("../escape.jpg", jpeg, "image/jpeg")),
])
why = {s["name"]: s["reason"] for s in r.json()["skipped"]}
assert why.get("notes.txt") == "not an image file type", why
assert why.get("fake.jpg") == "not a readable image", why
assert why.get("fresh.jpg") == "already in this folder", why
# The traversal is neutralised, not merely refused: the directory part is
# dropped and the file lands inside the destination like any other.
assert r.json()["saved"] == [str(UP / "escape.jpg")], r.json()
assert not (HERE / "escape.jpg").exists()

real_cap, uploads.MAX_MB = uploads.MAX_MB, 0.000001
r = c.post("/api/upload", data={"dir": str(UP)},
           files=[("files", ("big.jpg", jpeg, "image/jpeg"))])
uploads.MAX_MB = real_cap
assert r.json()["saved"] == [] and "larger than" in r.json()["skipped"][0]["reason"], r.json()

r = c.post("/api/upload", data={"dir": str(UP)},
           files=[("files", ("   ", jpeg, "image/jpeg")), ("files", (".hidden.jpg", jpeg, "image/jpeg"))])
assert r.json()["saved"] == [] and len(r.json()["skipped"]) == 2, r.json()
print("upload: 1 saved, rejects non-images, oversize, duplicates and nameless files")
shutil.rmtree(UP)

# --- T-12 / FR-30: with users configured, nothing works until you sign in ---
os.environ["LABEL_TOOL_USERS"] = f"alice:{auth.hash_password('hunter2')}"
try:
    locked = TestClient(app)
    assert locked.get("/api/config").status_code == 200          # public: UI needs it to boot
    assert locked.post("/api/session", json={"input_dir": POOL, "output_dir": str(OUT)}).status_code == 401
    assert locked.get("/api/auth/me").json() == {"enabled": True, "user": None}
    assert locked.post("/api/auth/login",
                       json={"username": "alice", "password": "wrong"}).status_code == 401
    assert locked.post("/api/auth/login",
                       json={"username": "mallory", "password": "hunter2"}).status_code == 401
    assert locked.post("/api/auth/login",
                       json={"username": "alice", "password": "hunter2"}).status_code == 200
    assert locked.get("/api/auth/me").json() == {"enabled": True, "user": "alice"}
    assert locked.post("/api/session",
                       json={"input_dir": POOL, "output_dir": str(OUT)}).status_code == 200

    # FR-31: the signed-in name lands on the instance this call creates
    assert locked.post("/api/label", json={
        "output_dir": str(OUT), "image": images[2],
        "boxes": [{"cls": "test_item", "box": [10, 10, 60, 60]}],
    }).status_code == 200
    meta = json.loads((OUT / "_bank" / "metadata.json").read_text(encoding="utf-8"))
    assert meta["instances"]["test_item"][-1]["labeled_by"] == "alice", meta["instances"]["test_item"][-1]

    locked.post("/api/auth/logout")
    assert locked.post("/api/session",
                       json={"input_dir": POOL, "output_dir": str(OUT)}).status_code == 401
    print("auth: gated, logged in as alice, labeled_by recorded, logged out")
finally:
    del os.environ["LABEL_TOOL_USERS"]

assert c.get("/api/auth/me").json() == {"enabled": False, "user": None}
assert c.post("/api/session", json={"input_dir": POOL, "output_dir": str(OUT)}).status_code == 200

shutil.rmtree(OUT)
shutil.rmtree(TEST)
print("SMOKE TEST OK")
