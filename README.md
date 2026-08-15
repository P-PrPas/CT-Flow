# YOLOE Visual-Prompt Labeler

Label a box → its SAVPE embedding goes into a prompt bank → the whole image
pool is rescored so you can watch confidence climb and decide when the model is
ready to auto-label. See `../docs/yoloe-auto-label-tool-design.md`.

No workspaces: **an input folder and an output folder is the whole config.**
The output folder *is* the project:

```
<output_dir>/
    labels/<stem>.txt   YOLO-format labels  <- the deliverable
    classes.txt         class index -> name
    _bank/embeddings.pt per-class list of per-instance embeddings
    _bank/metadata.json provenance + which images are done
```

## Layout

```
label_tool/
    backend/                FastAPI service
        app.py               mounts routers, that's it
        deps.py              checked_path() -- the path-safety check every router uses
        config.py            LABEL_TOOL_MODE / VM_ROOT / model path
        routers/             HTTP layer -- request in, service call, response out
            system.py          GET /api/config, /api/browse
            pool.py            session, image, boxes, label, relabel
            testset.py         /api/testset/*
            jobs.py            /api/jobs/{id}, score, evaluate, autolabel
        services/             business logic, no FastAPI/HTTP in here
            bank.py             the prompt bank (embeddings + labels for the pool)
            groundtruth.py      YOLO ground truth for the test set (no embeddings)
            yolo_labels.py      shared label-file read/write, used by both above
            images.py           list images in a folder
            vpe.py              YOLOE model wrapper (arm/predict_one/extract_embedding)
            metrics.py          precision/recall/F1 against ground truth
            job_tracker.py      in-memory progress tracking for background jobs
        Dockerfile
        requirements.txt
        _smoke_test.py        one runnable check, see Dev below
        test_pool/             fixture images for it
    frontend/                Next.js UI
        app/
            page.tsx             the one page -- state + layout, no fetch calls
            components/          BoxCanvas, DirPicker, EvalOverlay, ProgressBar
            lib/api.ts           every fetch call in the app lives here
            lib/types.ts         shapes shared between api.ts and page.tsx
            api/[...path]/       runtime proxy to the backend (API_URL env)
        Dockerfile
    docker-compose.yml
    certs/                    root CAs trusted at image-build time (see certs/README.md)
```

A router never touches a file directly and a service never imports FastAPI --
that split is what makes `backend/_smoke_test.py` able to hit the app through
`TestClient` while still calling `Bank(...)` directly to check what landed on
disk.

## Workflow

The UI has two tabs, **Label pool** and **Prepare test set**. They write to
different places and never share data — test images must stay held out, not
become prompts, or the F1 you read back is measuring memorization.

1. Open the pool once on **Label pool** (input = `<dataset>/pool`, any output)
   so its image list is loaded.
2. Switch to **Prepare test set**, point Test set at `<dataset>/test`, hit
   **Open test set**, then pull 10–20 images in from **Import from pool** —
   "Add random" or tick specific ones and "Add selected". This copies the
   files (the pool keeps its own copy; nothing there changes).
3. Draw ground-truth boxes on the imported images. Save writes straight to
   `test/labels/*.txt` + `test/classes.txt` via `groundtruth.py` — it never
   touches a prompt bank, since test images must stay held out. The **Test
   images** list underneath is the test-set manager: tick images (or Select
   all) and **Remove selected** deletes the file + its ground truth from
   `test/`. The pool's own copy is untouched — this only un-does the import.
4. Switch to **Label pool**: input = `<dataset>/pool` (or wherever the real
   images are), output = `<dataset>/out`. Labeling here extracts a SAVPE
   embedding per box into the prompt bank in `out/`.
5. Every so often hit **Evaluate on test set** — YOLOE runs on the held-out
   images with the current bank and reports precision / recall / F1 at IoU
   0.5, overall and per class. This is the readiness signal, not the pool
   confidence numbers (those only tell you which image to label next).
6. Hit **Eval report** (or "View full report →") for the detailed breakdown:
   every test image with ground truth and predictions drawn on top, color-coded
   by match status, so you can see *what kind* of mistake the model is making,
   not just the aggregate number.
7. F1 good enough? **Auto-label remaining** writes labels for the rest of the
   pool. They land in `labels/` like any other, tracked separately under
   `auto` in `_bank/metadata.json` so you can tell them from human ones.
8. Click into any auto-labeled image (marked "· reviewing auto-label") and it
   opens in **review mode**: the model's predicted boxes are fully editable —
   click one to select it, × to delete an over-prediction, drag to add a box
   the model missed. **Save review** rewrites just that image's label file
   (`/api/relabel`) with no embedding extraction, since fixing a prediction
   isn't the same as teaching the model a new prompt. Class choice is a
   dropdown of classes the bank already knows (a brand-new class has to come
   from Save to bank instead, on an unlabeled image).

Evaluate, Rescore, and Auto-label all run as background jobs with a progress
bar + ETA (`services/job_tracker.py` — poll `/api/jobs/{id}`, driven from the
frontend by `lib/api.ts`'s `runJob`), since each is a full inference pass over
a folder and can take a while. Revisiting an image you've already labeled (on
either tab) shows its saved boxes automatically, dimmed, so you always know
what's already been captured.

Save defaults to replacing an image's whole label file with whatever's
currently drawn. Tick **update (keep existing)** next to Save to add the
drawn boxes to what's already saved instead — for going back to an image and
adding one more instance without redrawing everything else. Works the same
way on both tabs (`mode: "replace" | "update"` on `/api/label` and
`/api/testset/label`; the merge itself is `services/yolo_labels.py`, shared by
`services/bank.py` and `services/groundtruth.py`).

`Bank.classes` is insertion-ordered, never alphabetically sorted — a label
file's class column is an index into that list, so it has to stay fixed once
assigned, or older files silently decode under the wrong class.

## Test datasets

All datasets live in `../data` (shared with the POC) and are mounted at `/data`
in the container — see `../data/README.md`. Two are ready to label:

- **`/data/iron_ore/pool`** — 235 frames of rock/ore on a moving belt. Input =
  `pool`, Output = `out`, and label ~15 images into `test/` first for a
  readiness metric. This is the one that matches the real use case.
- **`/data/conveyor_pvc`** — PVC fittings,
  [conveyor belt v3](https://universe.roboflow.com/onkar/conveyor-belt) CC BY
  4.0, already split into `pool/` `test/` `out/` with ground truth. Roboflow's
  class names are literally `0`/`1`; renamed to `defect` / `good_part` after
  eyeballing the crops, and the test split was restratified because roboflow's
  own val/test contain zero defects.

Measured baseline on `conveyor_pvc` (conf 0.15, IoU 0.5) — this dataset happens to contain one
easy class and one hard one, which is exactly what the readiness metric is for:

| prompts/class | good_part F1 | defect F1 |
|---|---|---|
| 1 | 0.78 | 0.04 |
| 5 | 0.74 | 0.04 |
| 20 | 0.80 | 0.07 |

`good_part` is auto-label-ready off a single prompt. `defect` (small chips and
scratches) never gets there — SAVPE at 640px can't separate them from clean
pipe. Checked that this is the model and not the plumbing: prompting from an
image and predicting on that same image scores 0.95, and `get_vpe` already
returns L2-normalized embeddings so re-normalizing the mean changes nothing.

## Run

```bash
cp .env.example .env      # point DATA_DIR at the folder holding your datasets
docker compose up --build # http://localhost:3000
```

Everything under `DATA_DIR` is mounted at `/data` in the api container, and
`/data` is the only thing the folder picker can reach — paths outside it are
rejected (the browser can send any string). Input, output, and test folders
all have to live under it.

## Multi-user (optional, off by default)

With no `LABEL_TOOL_USERS` there is no login, and `POST /api/upload` refuses to
run in `vm` mode — a shared server that takes files from anyone who knows the
URL is worse than no upload at all. Turn both on together:

```bash
.venv\Scripts\python.exe -m backend.services.auth alice 'their password'
# -> alice:pbkdf2$240000$...   put it in LABEL_TOOL_USERS (comma-separated)
python -c "import secrets; print(secrets.token_hex(32))"   # LABEL_TOOL_SECRET
```

Then every endpoint except `GET /api/config` needs a session cookie, and each
bank instance records the `labeled_by` who taught it. There is no login screen
in the UI yet — sign in with `POST /api/auth/login`.

`docker compose build` failing with `CERTIFICATE_VERIFY_FAILED`? Something is
intercepting TLS (corporate proxy, or antivirus HTTPS scanning). See
`certs/README.md`.

## Config

| env | default | meaning |
|---|---|---|
| `DATA_DIR` | `./data` | host folder mounted at `/data` |
| `WEB_PORT` | `3000` | port the UI is served on |
| `LABEL_TOOL_MODE` | `vm` in Docker | `vm` = confined to `LABEL_TOOL_VM_ROOT`; `local` = browse every drive |
| `LABEL_TOOL_VM_ROOT` | `/data` | the confinement root |
| `MODEL_PATH` | `/models/yoloe-11s-seg.pt` | weight baked into the api image |
| `LABEL_TOOL_USERS` | *(empty)* | `name:hash,name:hash` — empty means no login and no upload |
| `LABEL_TOOL_SECRET` | *(random per restart)* | signs the session cookie; unset = everyone is signed out on restart |
| `LABEL_TOOL_MAX_UPLOAD_MB` | `25` | per-file upload cap |
| `APP_UID` | `1000` | build arg — must own `DATA_DIR` on a Linux host, since the container is not root |

`local` mode only makes sense outside Docker, where the server runs on your own
PC and browsing the server means browsing your machine.

## Dev without Docker

Run from `label_tool/` (the parent of `backend/`) so `backend` resolves as a
package:

```bash
.venv\Scripts\python.exe -m uvicorn backend.app:app --port 8000 --reload
cd frontend && npm run dev          # needs API_URL if not 127.0.0.1:8000
.venv\Scripts\python.exe -m backend._smoke_test   # backend end-to-end check
.venv\Scripts\python.exe -m backend._experiment_conf 20   # T-01 conf sweep, needs data/conveyor_pvc
```

`_smoke_test` plus the `auth` / `events` / `metrics` self-checks are what CI
runs (`.github/workflows/backend.yml`).

CPU-only torch. For GPU, swap the base image and drop the
`--extra-index-url .../cpu` in `backend/Dockerfile`.
# CT-Flow
