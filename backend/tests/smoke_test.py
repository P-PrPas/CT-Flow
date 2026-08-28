"""One runnable check: open a session, label a box, verify the bank + the
annotations in PostgreSQL land and survive a reload, then rescore the pool.

It drives a running server over HTTP, which is the point -- this is the parity
harness for the Go port (docs/history/REFACTOR_PLAN.md phase 0), not only a Python test:

    SMOKE_BASE_URL=http://localhost:8000 python -m backend.tests.smoke_test

The same command drove FastAPI throughout the port and drives the Go service
now, which is what made "Go passes the same assertions Python does" one command
instead of a second suite that drifts.

Two things it needs from its environment:
  * the server must resolve the same filesystem paths this process does (run it
    on the host, or bind-mount so SMOKE_POOL means the same thing on both
    sides) -- input_dir crosses the wire as a plain string;
  * nothing else -- in particular NOT torch. Every assertion here goes over
    HTTP or reads a plain file; the checks that construct Bank() directly moved
    to backend/tests/bank_test.py so this can be driven from a machine that has no
    reason to carry a CUDA wheel.

Environment: SMOKE_BASE_URL, SMOKE_POOL, SMOKE_USER/SMOKE_PASSWORD,
DATABASE_URL.

Signing in is mandatory since T-27, so SMOKE_USER/SMOKE_PASSWORD are no longer
optional and the auth assertions are no longer skippable -- a server that does
not need them is a server that will not start.
"""
import json
import os
import shutil
import time
from pathlib import Path

import httpx

from . import dbcheck as _dbcheck

HERE = Path(__file__).parent
# input_dir is the only path a client ever sends -- the bank lives in a fixed
# .ctflow subfolder of it (see internal/platform/config); labels and test-set
# membership live in PostgreSQL, keyed by this same POOL path.
POOL = os.getenv("SMOKE_POOL") or str(HERE / "fixtures" / "pool")
# Scratch fixtures that get sent to the server as a path (input_dir, an
# upload destination) have to sit where the server is allowed to look, which
# is not necessarily next to this file -- a vm-mode target only accepts paths
# under its own root. Siblings of the pool always qualify.
SCRATCH = Path(POOL).parent
OUT = Path(POOL) / ".ctflow"
TEST = OUT / "testset"

BASE_URL = os.getenv("SMOKE_BASE_URL")
if not BASE_URL:
    raise SystemExit(
        "set SMOKE_BASE_URL to the API to drive, e.g.\n"
        "  SMOKE_BASE_URL=http://localhost:8000 python -m backend.tests.smoke_test"
    )
# 120s: an evaluate/autolabel pass over the fixture pool is a real inference
# run, and a cold model load on CPU is not fast.
c = httpx.Client(base_url=BASE_URL, timeout=120)

if OUT.exists():
    try:
        shutil.rmtree(OUT)
    except OSError as exc:
        # The prompt bank is written by the inference sidecar under its own uid.
        # Running the harness as someone who cannot delete it would silently
        # reuse the previous run's bank, and every class-count assertion below
        # would then be measuring the wrong thing -- so stop, and say how to fix
        # it, rather than producing a green run that proved nothing.
        raise SystemExit(
            f"cannot clear {OUT}: {exc}\n"
            f"It belongs to whichever uid the vpe service runs as (APP_UID). "
            f"Run the harness as that user, or point SMOKE_POOL at a fresh folder."
        )
_dbcheck.init_schema()
_dbcheck.delete_project(POOL)

# The server rejects every call below until we sign in, so this has to happen
# before the first request rather than in the auth section at the bottom.
SMOKE_USER = os.getenv("SMOKE_USER", "alice")
SMOKE_PASSWORD = os.getenv("SMOKE_PASSWORD", "hunter2")
r = c.post("/api/auth/login", json={"username": SMOKE_USER, "password": SMOKE_PASSWORD})
assert r.status_code == 200, (
    f"could not sign in as {SMOKE_USER}: {r.text}\n"
    "Signing in is mandatory (T-27) -- start the target server with "
    "LABEL_TOOL_USERS and point SMOKE_USER/SMOKE_PASSWORD at an entry in it."
)


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
# `mode` is gone with LABEL_TOOL_MODE (T-27) -- one behaviour, nothing to report.
assert "mode" not in cfg, cfg
assert cfg["default_model"] == "yoloe-11s-seg", cfg
assert {"id", "family", "size", "available"} <= cfg["models"][0].keys() and len(cfg["models"]) > 1, cfg["models"]
assert all("file" not in m for m in cfg["models"]), cfg["models"]  # no local paths leak to the browser
default_entry = next(m for m in cfg["models"] if m["id"] == cfg["default_model"])
assert default_entry["available"] is True, default_entry  # CI/dev always has the default weight cached

# --- FR-43 / FR-47: writing needs a project, and it has an owner ----------
# Before Phase 2 any endpoint could conjure a project row out of an input_dir,
# which left nameless, ownerless rows behind whenever anyone typo'd a path.
# Asserted through /api/testset/import, which is the shortest path from a
# request to a write: it reaches store.MarkTest without going through the
# inference sidecar first. /api/label would answer 400 "cannot read image"
# before the store is ever consulted, because the sidecar reads the image to
# extract an embedding -- a real refusal, just not this one.
NO_PROJECT = str(HERE / "fixtures" / "pool") + "-not-a-project"
r = c.post("/api/testset/import", json={"input_dir": NO_PROJECT, "images": []})
assert r.status_code == 404, (r.status_code, r.text)
assert r.json()["detail"] == "no project for this folder -- create it first", r.json()

r = c.post("/api/session", json={"input_dir": POOL})
assert r.status_code == 200, r.text
images = r.json()["images"]
assert len(images) == 3, images
assert r.json()["testset"] == {"images": [], "labeled": [], "classes": []}
# Opening a folder is what registers it, so the frontend needs no extra call and
# the row is never nameless: the folder names it, the opener owns it.
project = r.json()["project"]
PROJECT_ID = project["id"]
assert project["input_dir"] == POOL, project
assert project["name"] == Path(POOL).name, project
assert project["task_type"] == "detection", project
assert project["owner"]["oid"] == SMOKE_USER, project

# Re-opening adopts, and must not rename it or take it from its owner.
again = c.post("/api/session", json={"input_dir": POOL}).json()["project"]
assert again["id"] == PROJECT_ID and again["name"] == project["name"], again

# Creating over an existing folder is refused, and says whose work it is.
r = c.post("/api/projects", json={"name": "Second claim", "input_dir": POOL})
assert r.status_code == 409, (r.status_code, r.text)
assert project["name"] in r.json()["detail"], r.json()

r = c.post("/api/projects", json={"name": "x", "input_dir": POOL, "task_type": "segmentation"})
assert r.status_code == 400 and r.json()["detail"] == "unknown task type: segmentation", r.text

r = c.get(f"/api/projects/{PROJECT_ID}")
assert r.status_code == 200 and r.json()["project"]["input_dir"] == POOL, r.text
assert c.get(f"/api/projects/{PROJECT_ID + 10_000}").status_code == 404
print("pool:", len(images), "images ·", f"project {PROJECT_ID} {project['name']!r}")

target = images[0]
r = c.post("/api/label", json={
    "input_dir": POOL,
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
    "input_dir": POOL, "image": target,
    "boxes": [{"cls": "test_item", "box": [1, 1, 5, 5]}],
    "model_id": "yoloe-11m-seg",
})
assert r.status_code == 409, r.text
print("model lock: mismatched model_id on an existing bank rejected (409)")

saved0 = _dbcheck.read_boxes(POOL, "pool", target)
assert saved0 and saved0[0]["cls"] == "test_item", saved0
print("annotation stored in DB:", saved0)

# reload from disk -- the bank must round-trip (embeddings/classes are still a
# file). A fresh /api/session re-reads it from scratch, which is what proves it:
# nothing is cached between requests on either side of the sidecar boundary.
def bank_classes() -> list[dict]:
    """BankSummary's `classes`: [{"name", "count"}] in insertion order. Counts
    matter as much as names here -- "relabel added no embeddings" is a
    statement about counts, not about the class list."""
    return c.post("/api/session", json={"input_dir": POOL}).json()["bank"]["classes"]


def bank_names() -> list[str]:
    return [entry["name"] for entry in bank_classes()]


assert bank_names() == ["test_item"]
assert _dbcheck.list_by_status(POOL, "pool")["labeled"] == [target]

r = c.post("/api/score", json={"input_dir": POOL, "images": images[1:]})
assert r.status_code == 200, r.text
scores = wait_job(r.json()["job_id"])["scores"]
assert set(scores) == set(images[1:]), scores
# FR-18: every score carries an 8x8 thumbnail the UI uses to spread picks
assert all(len(s["sig"]) == 64 for s in scores.values()), scores
print("scores:", {p: (s["conf"], s["cls"]) for p, s in scores.items()})

# a saved image should hand back its own boxes on request
r = c.get("/api/boxes", params={"input_dir": POOL, "image": target})
assert r.status_code == 200, r.text
saved = r.json()["boxes"]
assert saved and saved[0]["cls"] == "test_item", saved
print("saved boxes:", saved)

# mode="update": add a box without losing the one already there
r = c.post("/api/label", json={
    "input_dir": POOL, "image": target,
    "boxes": [{"cls": "test_item", "box": [200, 200, 260, 260]}],
    "mode": "update",
})
assert r.status_code == 200, r.text
r = c.get("/api/boxes", params={"input_dir": POOL, "image": target})
merged = r.json()["boxes"]
assert len(merged) == 2, merged  # original box survived, new one added
print("after update:", merged)

# mode="replace" (default) must still fully overwrite
r = c.post("/api/label", json={
    "input_dir": POOL, "image": target,
    "boxes": [{"cls": "test_item", "box": [5, 5, 15, 15]}],
})
assert r.status_code == 200, r.text
r = c.get("/api/boxes", params={"input_dir": POOL, "image": target})
replaced = r.json()["boxes"]
assert len(replaced) == 1, replaced
print("after replace:", replaced)

# class-order stability: a new class name that sorts before "test_item"
# must not shift the index test_item was already written under
r = c.post("/api/label", json={
    "input_dir": POOL, "image": images[1],
    "boxes": [{"cls": "aaa_new_class", "box": [1, 1, 20, 20]}],
})
assert r.status_code == 200, r.text
r = c.get("/api/boxes", params={"input_dir": POOL, "image": target})
still_correct = r.json()["boxes"]
assert still_correct and still_correct[0]["cls"] == "test_item", still_correct
# T-21: the DB-backed class registry (the FastAPI annotations_db module) must keep
# the same append-only ordering the old classes.txt guaranteed.
assert _dbcheck.get_classes(POOL, "pool") == ["test_item", "aaa_new_class"], \
    _dbcheck.get_classes(POOL, "pool")
print("class order stable after adding 'aaa_new_class':", still_correct)

# /api/relabel: fix a generated label directly -- no new embeddings, unlike /api/label
bank_before = bank_classes()
r = c.post("/api/relabel", json={
    "input_dir": POOL, "image": target,
    "boxes": [{"cls": "test_item", "box": [1, 1, 9, 9]}],
})
assert r.status_code == 200, r.text
assert r.json()["bank"]["classes"] == bank_before, r.json()["bank"]  # no embeddings added
r = c.get("/api/boxes", params={"input_dir": POOL, "image": target})
assert [b["cls"] for b in r.json()["boxes"]] == ["test_item"], r.json()

# deleting every box (boxes=[]) is a legitimate "model was wrong here"
r = c.post("/api/relabel", json={"input_dir": POOL, "image": target, "boxes": []})
assert r.status_code == 200, r.text
r = c.get("/api/boxes", params={"input_dir": POOL, "image": target})
assert r.json()["boxes"] == [], r.json()

# a class never taught to the bank must be rejected, not silently mis-indexed
r = c.post("/api/relabel", json={
    "input_dir": POOL, "image": target,
    "boxes": [{"cls": "never_seen_before", "box": [1, 1, 9, 9]}],
})
assert r.status_code == 400, r.text
print("relabel: bank untouched by review edits, empty boxes allowed, unknown class rejected")


# --- readiness loop: prepare a test set (ground truth only, no prompt bank) ---
# No file copy and no second folder: a test image is a pool image flagged in
# a manifest (see tools/groundtruth.py) -- images[1] IS the test image.
r = c.post("/api/testset/import", json={"input_dir": POOL, "images": [images[1]]})
assert r.status_code == 200, r.text
test_img = r.json()["images"][0]
assert r.json()["imported"] == [test_img]
assert test_img == images[1]  # no copy -- the pool image itself is the test image
assert Path(images[1]).exists()  # pool untouched (there was never a second file)

# re-importing the same source must not clobber anything
r = c.post("/api/testset/import", json={"input_dir": POOL, "images": [images[1]]})
assert r.json()["imported"] == []

# the isolation invariant: a flagged image can never be taught to the bank
r = c.post("/api/label", json={
    "input_dir": POOL, "image": test_img,
    "boxes": [{"cls": "test_item", "box": [1, 1, 9, 9]}],
})
assert r.status_code == 400, r.text
r = c.post("/api/relabel", json={"input_dir": POOL, "image": test_img, "boxes": []})
assert r.status_code == 400, r.text
print("isolation: a test-flagged image is rejected by /api/label and /api/relabel")

# and the mirror invariant: ground truth can't be written for an unflagged image
r = c.post("/api/testset/label", json={
    "input_dir": POOL, "image": images[2],
    "boxes": [{"cls": "test_item", "box": [1, 1, 9, 9]}],
})
assert r.status_code == 400, r.text

r = c.post("/api/testset/label", json={
    "input_dir": POOL,
    "image": test_img,
    "boxes": [{"cls": "test_item", "box": [40, 40, 150, 150]}],
})
assert r.status_code == 200, r.text
assert r.json()["classes"] == ["test_item"]
print("ground truth:", r.json())

# mode="update" on ground truth too: add without losing what's there
r = c.post("/api/testset/label", json={
    "input_dir": POOL, "image": test_img,
    "boxes": [{"cls": "test_item", "box": [5, 5, 30, 30]}],
    "mode": "update",
})
assert r.status_code == 200, r.text
r = c.get("/api/boxes", params={"input_dir": POOL, "image": test_img, "kind": "test"})
gt_merged = r.json()["boxes"]
assert len(gt_merged) == 2, gt_merged
print("ground truth after update:", gt_merged)

# must not touch the prompt bank -- test images are held out, not prompts
assert not (TEST / "_bank").exists()
assert bank_classes() == bank_before  # unaffected by testset writes

r = c.post("/api/testset/import", json={"input_dir": POOL, "images": []})  # cheap way to re-read state
assert r.json()["labeled"] == [Path(test_img).stem]

# test-set manager: flag a second image, then unflag it -- manifest entry +
# label gone, the first image and both pool sources untouched
r = c.post("/api/testset/import", json={"input_dir": POOL, "images": [images[2]]})
assert r.status_code == 200, r.text
second_img = r.json()["imported"][0]
r = c.post("/api/testset/label", json={
    "input_dir": POOL, "image": second_img,
    "boxes": [{"cls": "test_item", "box": [10, 10, 50, 50]}],
})
assert r.status_code == 200, r.text

r = c.post("/api/testset/remove", json={"input_dir": POOL, "images": [second_img]})
assert r.status_code == 200, r.text
assert r.json()["removed"] == [second_img]
assert r.json()["images"] == [test_img]
assert Path(second_img).exists()  # unflagging never deletes -- there was no copy to delete
assert not (TEST / "labels" / f"{Path(second_img).stem}.txt").exists()
print("removed from test set:", r.json()["removed"])

r = c.post("/api/evaluate", json={"input_dir": POOL, "conf": 0.1})
assert r.status_code == 200, r.text
assert r.json()["total"] == 1
ev = wait_job(r.json()["job_id"])
assert ev["images"] == 1 and 0.0 <= ev["overall"]["f1"] <= 1.0, ev
img0 = ev["per_image"][0]
assert set(img0) >= {"image", "gt", "pred", "tp", "fp", "fn"}, img0
assert all("matched" in g for g in img0["gt"]), img0
print("eval:", ev["overall"], "| per-image gt/pred:", len(img0["gt"]), len(img0["pred"]))

r = c.post("/api/autolabel", json={"input_dir": POOL, "images": images[1:], "conf": 0.1})
assert r.status_code == 200, r.text
auto = wait_job(r.json()["job_id"])
assert auto["written"] + auto["no_detection"] == 2, auto
assert len(auto["bank"]["auto"]) == auto["written"]
# FR-28: the empty ones are named, not just counted
assert len(auto["no_detection_images"]) == auto["no_detection"], auto
print("autolabel:", auto["written"], "written,", auto["no_detection"], "empty")

# FR-19: pre-annotation for a single image, straight from the bank
r = c.post("/api/predict", json={"input_dir": POOL, "image": target, "conf": 0.05})
assert r.status_code == 200, r.text
drafts = r.json()["boxes"]
assert all(d["cls"] in bank_names() and len(d["box"]) == 4 for d in drafts), drafts
print("predict:", len(drafts), "draft box(es)")

# an empty bank must cost nothing rather than error
EMPTY_POOL = SCRATCH / "_smoke_empty"
if EMPTY_POOL.exists():
    shutil.rmtree(EMPTY_POOL)
r = c.post("/api/predict", json={"input_dir": str(EMPTY_POOL), "image": target})
assert r.status_code == 200 and r.json()["boxes"] == [], r.text
shutil.rmtree(EMPTY_POOL)

# FR-09 / T-17: relabel mode="update" merges instead of replacing
c.post("/api/relabel", json={"input_dir": POOL, "image": target, "boxes": []})
for box in ([1, 1, 9, 9], [20, 20, 40, 40]):
    r = c.post("/api/relabel", json={
        "input_dir": POOL, "image": target,
        "boxes": [{"cls": "test_item", "box": box}], "mode": "update",
    })
    assert r.status_code == 200, r.text
merged_review = c.get("/api/boxes", params={"input_dir": POOL, "image": target}).json()["boxes"]
assert len(merged_review) == 2, merged_review
print("relabel update:", len(merged_review), "boxes kept")

# T-07: evaluate history round-trips through <input_dir>/.ctflow/_bank/eval_history.json
point = {"ts": 1, "conf": 0.1, "prompts": {"test_item": 3}, "totalPrompts": 3,
         "overall": ev["overall"], "perClass": ev["per_class"]}
assert c.post("/api/history", json={"input_dir": POOL, "point": point}).json()["history"] == [point]
assert c.get("/api/history", params={"input_dir": POOL}).json()["history"] == [point]
assert (OUT / "_bank" / "eval_history.json").exists()
assert c.request("DELETE", "/api/history", params={"input_dir": POOL}).json()["history"] == []
assert c.get("/api/history", params={"input_dir": POOL}).json()["history"] == []
print("eval history: persisted, read back, cleared")


# Inference has to survive the bank gaining a class mid-process (it grew from
# 1 to 2 above). A threshold this low guarantees detections, which is what it
# takes to reach the mask head where a stale class count blows up -- see arm().
r = c.post("/api/predict", json={"input_dir": POOL, "image": target, "conf": 0.001})
assert r.status_code == 200 and r.json()["boxes"], r.text[:200]
assert {b["cls"] for b in r.json()["boxes"]} <= set(bank_names()), r.json()
print("predict after a class was added:", len(r.json()["boxes"]), "boxes, no shape error")

# FR-33: a per-class threshold above 1.0 can never be met, so naming every
# class in conf_by_class must empty the result even at conf=0.0
r = c.post("/api/predict", json={
    "input_dir": POOL, "image": target, "conf": 0.0,
    "conf_by_class": {n: 1.01 for n in bank_names()},
})
assert r.status_code == 200 and r.json()["boxes"] == [], r.text
r = c.post("/api/evaluate", json={
    "input_dir": POOL, "conf": 0.1,
    "conf_by_class": {"test_item": 0.9},
})
assert wait_job(r.json()["job_id"])["conf_by_class"] == {"test_item": 0.9}
print("conf_by_class: per-class thresholds honoured and echoed back")

# --- FR-31: every bank instance records who taught it (None when auth is off)
meta = json.loads((OUT / "_bank" / "metadata.json").read_text(encoding="utf-8"))
assert all("labeled_by" in i for insts in meta["instances"].values() for i in insts), meta


# --- FR-44 / FR-50: what the home page reads --------------------------------
# Counts come from images.status, and "who worked here" is derived from
# annotations.created_by rather than a membership list -- so both have to
# reflect the labeling this run actually did.
listed = c.get("/api/projects").json()["projects"]
mine = next(p for p in listed if p["input_dir"] == POOL)
assert mine["id"] == PROJECT_ID, mine
assert mine["owner"]["oid"] == SMOKE_USER, mine
# Compared against the session's own lists rather than a fixed number: both
# read images.status, so this checks the card agrees with the workspace --
# including the kind='pool' filter, which is where a miscount would come from.
# A fixed ">= 1 auto" would instead be asserting that YOLOE detects something
# in these fixtures, which it does not and was never meant to.
state = c.post("/api/session", json={"input_dir": POOL}).json()["bank"]
assert mine["labeled"] == len(state["labeled"]), (mine, state["labeled"])
assert mine["auto"] == len(state["auto"]), (mine, state["auto"])
assert mine["labeled"] >= 1, "the run hand-labeled at least one image"
# A local login has no users row, so the stored subject is the best name there
# is -- an unreadable contributor still beats a missing one.
assert [m["oid"] for m in mine["contributors"]] == [SMOKE_USER], mine["contributors"]
assert mine["contributors"][0]["boxes"] >= 1, mine["contributors"]

r = c.patch(f"/api/projects/{PROJECT_ID}", json={"name": "Renamed by smoke"})
assert r.status_code == 200 and r.json()["project"]["name"] == "Renamed by smoke", r.text
assert r.json()["project"]["owner"]["oid"] == SMOKE_USER, "a rename must not clear the owner"
assert c.patch(f"/api/projects/{PROJECT_ID}", json={}).status_code == 400
print("projects:", len(listed), "listed ·", mine["labeled"], "labeled /", mine["auto"],
      "auto · agrees with the session")

# --- §7: effort metrics outlive the browser tab ---
for e in [{"kind": "session", "session": "s1"},
          {"kind": "label", "session": "s1", "secs": 12},
          {"kind": "auto", "session": "s1", "secs": 300, "written": 4},
          {"kind": "fix", "session": "s1"}]:
    assert c.post("/api/events", json={"input_dir": POOL} | e).status_code == 200
stats = c.get("/api/events", params={"input_dir": POOL}).json()["summary"]
assert stats["sessions"] == 1 and stats["abandonment"] == 0.0, stats
assert stats["median_label_secs"] == 12 and stats["correction_rate"] == 0.25, stats
assert (OUT / "_bank" / "events.jsonl").exists()
print("events:", stats)

# --- FR-29 / T-13: upload ---
UP = SCRATCH / "_smoke_upload"
if UP.exists():
    shutil.rmtree(UP)
jpeg = Path(images[0]).read_bytes()

# T-13's precondition -- no upload on a shared server without a login -- used
# to be a branch here, because the target could have been started either way.
# It cannot be any more (T-27): the server refuses to start without a login,
# so the refusing half was unreachable and only the accepting half is left.
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
assert not (SCRATCH / "escape.jpg").exists()

# The per-file size cap. There is nothing to reach into over HTTP, so the run
# has to be configured with a small cap and we send something bigger than it.
# Both sides read LABEL_TOOL_MAX_UPLOAD_MB from the environment, so harness
# and server agree on the number without a new endpoint to ask over.
if float(os.getenv("LABEL_TOOL_MAX_UPLOAD_MB", "25")) <= 2:
    cap_bytes = int(float(os.environ["LABEL_TOOL_MAX_UPLOAD_MB"]) * 1024 * 1024)
    # A real JPEG header followed by padding: the size check has to reject this
    # before the decode check gets a chance to, which is the ordering under test.
    oversize = jpeg + b"\0" * (cap_bytes + 1)
    r = c.post("/api/upload", data={"dir": str(UP)},
               files=[("files", ("big.jpg", oversize, "image/jpeg"))])
    assert r.json()["saved"] == [] and "larger than" in r.json()["skipped"][0]["reason"], r.json()
else:
    print("upload: size-cap check skipped -- set LABEL_TOOL_MAX_UPLOAD_MB=1 on "
          "both server and harness to exercise it over HTTP")

r = c.post("/api/upload", data={"dir": str(UP)},
           files=[("files", ("   ", jpeg, "image/jpeg")), ("files", (".hidden.jpg", jpeg, "image/jpeg"))])
assert r.json()["saved"] == [] and len(r.json()["skipped"]) == 2, r.json()
print("upload: 1 saved, rejects non-images, oversize, duplicates and nameless files")
shutil.rmtree(UP)

# --- FR-30 / FR-47: nothing works until you sign in ------------------------
# This block never ran in CI before T-28: it was gated on the target server
# having LABEL_TOOL_USERS set, and the workflow did not set it. That is why the
# exact-match assertions below still expected an /api/auth/me without `mode`
# months after the field was added. CI now starts the server with local users,
# so this runs on every push.
def check_auth(client, user: str, password: str, wrong_user: str = "mallory"):
    assert client.get("/api/config").status_code == 200          # public: UI needs it to boot
    assert client.post("/api/auth/logout").status_code == 200
    assert client.post("/api/session", json={"input_dir": POOL}).status_code == 401
    # enabled is always True since T-27 -- there is no server without a login.
    # oid travels with user: signed out means both are null.
    assert client.get("/api/auth/me").json() == {
        "enabled": True, "user": None, "oid": None, "mode": "local"}
    assert client.post("/api/auth/login",
                       json={"username": user, "password": "wrong"}).status_code == 401
    assert client.post("/api/auth/login",
                       json={"username": wrong_user, "password": password}).status_code == 401
    assert client.post("/api/auth/login",
                       json={"username": user, "password": password}).status_code == 200
    # `oid` is the caller's own attribution key -- the value that lands in
    # projects.owner_oid and annotations.created_by, and what the home page
    # compares to split "yours" from everyone else's. A local account has no
    # separate subject, so it equals the username here; under OIDC it is the
    # provider's `sub` and the display name is a different string entirely.
    # That difference is why the UI cannot answer "is this mine" from `user`.
    me = client.get("/api/auth/me").json()
    assert me == {"enabled": True, "user": user, "oid": user, "mode": "local"}
    assert client.post("/api/auth/login",
                       json={"username": user, "password": password}).json() == me
    assert client.post("/api/session", json={"input_dir": POOL}).status_code == 200

    # FR-31: the signed-in name lands on the instance this call creates
    assert client.post("/api/label", json={
        "input_dir": POOL, "image": images[2],
        "boxes": [{"cls": "test_item", "box": [10, 10, 60, 60]}],
    }).status_code == 200
    meta = json.loads((OUT / "_bank" / "metadata.json").read_text(encoding="utf-8"))
    assert meta["instances"]["test_item"][-1]["labeled_by"] == user, meta["instances"]["test_item"][-1]

    client.post("/api/auth/logout")
    assert client.post("/api/session", json={"input_dir": POOL}).status_code == 401
    print(f"auth: gated, logged in as {user}, labeled_by recorded, logged out")


check_auth(c, SMOKE_USER, SMOKE_PASSWORD)
c.post("/api/auth/login", json={"username": SMOKE_USER, "password": SMOKE_PASSWORD})

assert c.post("/api/session", json={"input_dir": POOL}).status_code == 200

# --- FR-43: deleting a project takes the rows and leaves the files ----------
# Last, because everything above needs the project to exist. The promise the UI
# makes on this button is that the dataset survives, so that is what is checked.
r = c.delete(f"/api/projects/{PROJECT_ID}")
assert r.status_code == 200, r.text
assert r.json()["kept_on_disk"] == POOL, r.json()
assert Path(POOL).is_dir() and len(list(Path(POOL).glob("*.jpg"))) > 0, "delete removed image files"
assert (OUT / "_bank" / "metadata.json").exists(), "delete removed the prompt bank"
assert c.get(f"/api/projects/{PROJECT_ID}").status_code == 404
assert c.delete(f"/api/projects/{PROJECT_ID}").status_code == 404
# And with the project gone, writing to the folder is refused again. Same
# endpoint as the check at the top, and for the same reason.
assert c.post("/api/testset/import",
              json={"input_dir": POOL, "images": []}).status_code == 404
print("projects: deleted, dataset and prompt bank untouched")

# Teardown must not be able to fail the run. The prompt bank is written by the
# inference sidecar, which runs as its own uid, so whoever drives the harness
# may simply not be allowed to delete it -- that says nothing about whether the
# assertions above passed.
try:
    shutil.rmtree(OUT)
except OSError as exc:
    print(f"note: could not remove {OUT} ({exc}) -- left for the owning user to clean up")
_dbcheck.delete_project(POOL)
print("SMOKE TEST OK")
