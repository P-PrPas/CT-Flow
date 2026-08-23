# CT-Flow

![backend CI](https://github.com/P-PrPas/CT-Flow/actions/workflows/backend.yml/badge.svg)
![go](https://img.shields.io/badge/go-1.26-00ADD8?logo=go&logoColor=white)
![python](https://img.shields.io/badge/python-3.12-3776AB?logo=python&logoColor=white)
![next.js](https://img.shields.io/badge/next.js-15.5-000000?logo=nextdotjs&logoColor=white)
![docker](https://img.shields.io/badge/docker-compose-2496ED?logo=docker&logoColor=white)
![license](https://img.shields.io/badge/license-internal-lightgrey)

**Teach a model by drawing one box.** CT-Flow is a human-in-the-loop visual-prompt
labeling tool for [YOLOE](https://github.com/THU-MIG/yoloe): label a box, its
SAVPE embedding drops into a per-class prompt bank, the whole image pool gets
rescored instantly, and a held-out test set tells you — with an actual
precision/recall/F1 number, not a guess — when the model is ready to
auto-label the rest.

No project workspaces, no training run, no label taxonomy to pre-declare.
**One image folder is the whole config** — labels, the prompt bank, and the
test set you hold back all live in a hidden `.ctflow/` subfolder inside it.

---

## Contents

- [How it works](#how-it-works)
- [Feature status](#feature-status)
- [Repository layout](#repository-layout)
- [Quick start (Docker)](#quick-start-docker)
- [Using CT-Flow](#using-ct-flow)
- [Model selection](#model-selection)
- [Keyboard shortcuts](#keyboard-shortcuts)
- [Multi-user & security](#multi-user--security-optional)
- [Configuration reference](#configuration-reference)
- [API overview](#api-overview)
- [Local development](#local-development-without-docker)
- [Testing & CI](#testing--ci)
- [Datasets & measured accuracy](#datasets--measured-accuracy)
- [Known limitations & roadmap](#known-limitations--roadmap)
- [Documentation index](#documentation-index)
- [Troubleshooting](#troubleshooting)

---

## How it works

Every image labeled feeds an embedding back into the model, so the model gets
better *while* the pool is being labeled — not after a separate training step.

```mermaid
flowchart LR
    A["Draw one box"] --> B["Extract SAVPE\nembedding"]
    B --> C[("Prompt bank\n_bank/embeddings.pt")]
    C --> D["Rescore the\nwhole pool"]
    D -->|confidence rises| A
    C --> E["Evaluate on\nheld-out test set"]
    E -->|"F1 ≥ 0.75"| F["Auto-label\nremaining pool"]
    E -->|"F1 too low"| A
    F --> G["Review mode\nfix model mistakes"]
    G -.->|edits teach nothing new| F
```

The test set never touches the prompt bank — it exists only to answer "is
this ready?" with a number, so that question isn't answered by eyeballing the
pool.

## Feature status

| Area | Status | Notes |
|---|---|---|
| Label → embed → rescore loop | Ready | the core workflow, fully wired end to end |
| Held-out evaluation (precision/recall/F1 @ IoU 0.5) | Ready | per class + overall, with a visual report |
| Per-class confidence thresholds (`conf_by_class`) | Ready | needed once one class is easy and another is hard — see [datasets](#datasets--measured-accuracy) |
| Selectable YOLOE checkpoint (`backend/models.json`) | Ready | 11 checkpoints, `yoloe-v8s-seg` up to `yoloe-26x-seg` — locked per project after the first label, see [model selection](#model-selection) |
| GPU (CUDA) inference | Ready | on by default in Docker; falls back to CPU with a one-line build-arg override |
| Auto-label + review mode | Ready | predicted boxes are fully editable before they're accepted |
| Learning-curve / plateau advice | Ready | "keep labeling" vs "diminishing returns" per class |
| Session auth (`/api/auth/*`) | Backend only | off unless `LABEL_TOOL_USERS` is set; **no sign-in screen in the UI yet** |
| Go backend | Ready | the API is Go; only YOLOE inference and the prompt bank are still Python, see [repository layout](#repository-layout) |
| Image upload | Backend only | `POST /api/upload`, refuses to run without auth in `vm` mode; no dropzone in the UI yet |
| Per-label attribution (`labeled_by`) | Ready | recorded once auth is on |
| Usage metrics (`_bank/events.jsonl`) | Backend only | abandonment / correction-rate math is ready; nothing calls `POST /api/events` from the UI yet |
| Embedding-distance duplicate detection | Approximated | an 8×8 thumbnail hash stands in for true embedding distance — good enough today, see [limitations](#known-limitations--roadmap) |

## Repository layout

```
label_tool/                     repo root
├── backend/                    the whole backend: Go API + Python inference sidecar
│   ├── go.mod                    module root -- Go lives under backend/, like everything else backend
│   ├── cmd/api/main.go           the API binary: env -> deps -> routes -> serve
│   ├── internal/                 layered, so what a package *is* is visible from its path
│   │   ├── transport/httpapi/      HTTP only: handlers, middleware, request/response shapes
│   │   │     system.go               GET /api/config, /api/browse, /api/image
│   │   │     pool.go · label.go      session, boxes, label, relabel, predict
│   │   │     testset.go              /api/testset/*
│   │   │     jobs.go                 /api/jobs/{id}, score, evaluate, autolabel, reembed
│   │   │     auth.go                 /api/auth/*  + the login middleware
│   │   │     upload.go · export.go   POST /api/upload, GET /api/export
│   │   │     project.go              history + events
│   │   ├── core/                   pure logic, no I/O -- testable with plain values
│   │   │   ├── metrics/              precision/recall/F1 at IoU 0.5
│   │   │   └── export/               YOLO / COCO / Pascal VOC writers
│   │   ├── infra/                  adapters over something outside the process
│   │   │   ├── store/                PostgreSQL: projects/classes/images/annotations
│   │   │   ├── vpe/                  client for the inference sidecar
│   │   │   ├── events/               append-only usage-metrics log
│   │   │   ├── history/              eval-run history behind the learning curve
│   │   │   └── images/               listing images in a folder
│   │   ├── platform/               cross-cutting, used by every layer
│   │   │   ├── config/               env + PathAllowed, the path-safety gate
│   │   │   ├── auth/                 pbkdf2 hashing + signed session cookies
│   │   │   ├── jobs/                 in-memory progress tracking
│   │   │   └── models/               the selectable-checkpoint catalog
│   │   └── testsupport/            locating shared fixtures from any package depth
│   ├── inference/                the Python sidecar -- everything that needs torch
│   │   ├── service.py              its six endpoints (JSON, and NDJSON for long passes)
│   │   ├── vpe.py                  YOLOE wrapper: armed() / predict_one() / extract_embedding()
│   │   ├── bank.py                 the prompt bank (embeddings + labeled_by provenance)
│   │   ├── models.py               resolves a model_id to a weight file
│   │   ├── requirements.txt
│   │   └── Dockerfile
│   ├── tools/                    one-off scripts, and the code only they still use
│   │   ├── experiment_conf.py      T-01 conf-threshold sweep, see Datasets below
│   │   └── metrics.py · groundtruth.py · yolo_labels.py
│   ├── tests/                    the harness -- ships in no image
│   │   ├── smoke_test.py           the end-to-end check, see Testing below
│   │   ├── parity.py               diffs two running backends, see Testing below
│   │   ├── bank_test.py            prompt-bank unit checks
│   │   ├── dbcheck.py              read-only PostgreSQL assertions
│   │   ├── gen_testdata.py         the cross-language golden vectors
│   │   ├── testdata/ · fixtures/pool/
│   │   └── requirements.txt
│   ├── db/schema.sql             applied by the API at boot; idempotent
│   ├── models.json               checkpoint catalog, read by both services
│   └── Dockerfile                the Go API image (~48 MB)
```

**Why two backends.** Everything that needs torch — YOLOE inference and the
prompt bank — is a Python sidecar; everything else is Go. That line is not
negotiable in either direction: YOLOE's SAVPE head has no Go equivalent, and
`embeddings.pt` is a `torch.save`, so the bank and the model cannot be split
across the boundary (`Bank.lock_model()` and `reembed()` commit atomically under
one file lock, which only works with a single writer). In exchange the sidecar
owns nothing else: no database, no uploads, no auth. Full reasoning and the
migration record: [`docs/REFACTOR_PLAN.md`](docs/REFACTOR_PLAN.md).

A handler never touches the database directly and no package under `internal/`
imports `net/http` except `internal/transport/httpapi` — that split is what lets
`backend/tests/smoke_test.py` drive the whole app over HTTP while `internal/infra/store`'s
tests hit a real PostgreSQL to check what actually landed. The same split exists
on the frontend: `session.ts` owns every mutation, panels only render a slice of
it — a panel takes one `s` prop instead of forty.

## Quick start (Docker)

```bash
cp .env.example .env      # point DATA_DIR at the folder holding your datasets,
                           # and set POSTGRES_PASSWORD (required -- compose won't start without it)

# Model weights are gitignored, and backend/inference/Dockerfile bakes three of
# them into the image, so a fresh clone has to fetch them once before building.
# Ultralytics resolves each filename to its own download URL:
pip install ultralytics
mkdir -p models && cd models && python -c "from ultralytics import YOLOE
for m in ('yoloe-11s-seg', 'yoloe-26s-seg', 'yoloe-26x-seg'): YOLOE(m + '.pt')" && cd ..

docker compose up --build # http://localhost:3000
```

Everything under `DATA_DIR` is mounted at `/opt/mount/project` in the api
container, and `/opt/mount/project` is the only thing the folder picker can
reach — paths outside it are rejected server-side (the browser can send any
string). Model weights land
in a named volume (`models`) so they persist across restarts; the default
checkpoint plus the two newest/largest ones are baked into the image so a
brand-new volume starts pre-seeded (see [Model
selection](#model-selection)), everything else in the catalog auto-downloads
into the volume the first time it's actually selected.

> **GPU by default.** The build installs a CUDA build of PyTorch and
> `docker-compose.yml` requests a GPU for the `api` service. This needs an
> NVIDIA GPU + driver + the [NVIDIA Container
> Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html)
> on the host (`docker info` should list an `nvidia` runtime). No GPU on this
> machine? Either delete the `deploy.reservations` block in
> `docker-compose.yml`, or build CPU-only without touching any file:
> ```bash
> docker compose build --build-arg TORCH_INDEX_URL=https://download.pytorch.org/whl/cpu api
> ```

## Using CT-Flow

The UI has four tabs. **Label** and **Test set** write to different places
and never share data — test images must stay held out, or the F1 you read
back is measuring memorization, not generalization.

1. **Label** — set the image folder (`<dataset>/pool`) once in the session
   setup card; that's the only folder there is to pick. Draw a box, name the
   class, save. That save extracts a SAVPE embedding into the prompt bank at
   `<dataset>/pool/.ctflow/_bank/`.
2. **Test set** — pull 10–20 images in with **Import from pool** ("Add
   random" or tick specific ones). This flags them as a separate row in
   PostgreSQL, no file copy — a test image *is* the pool image, so there's
   nothing to duplicate on disk. Draw ground-truth boxes the same way — Save
   here writes straight to PostgreSQL, never into a prompt bank; the backend
   rejects any attempt to teach the bank from a flagged image with a `400`.
3. Hit **Evaluate on test set** (from either tab) — YOLOE runs against the
   held-out images with the current bank and reports precision / recall / F1
   at IoU 0.5, overall and per class. This is the readiness signal, not pool
   confidence, which only tells you *which* image to label next.
4. **Report** tab — every test image with ground truth and predictions drawn
   on top, color-coded by match status, so you can see *what kind* of
   mistake the model is making.
5. **Progress** tab — F1 vs. number of examples taught, one line per class,
   plus plain-language advice: keep labeling this class, or it's plateaued
   and more hand-labeling has little left to buy (bar is F1 ≥ 0.75).
6. F1 good enough? **Auto-label remaining** writes labels for the rest of the
   pool, tracked separately (`auto` status in PostgreSQL) so you can tell
   them from human-drawn ones.
7. Click into any auto-labeled image ("reviewing auto-label") to enter
   **review mode** — every predicted box is editable: click to select, × to
   delete an over-prediction, drag to add a box the model missed. **Save
   review** rewrites just that image's label in PostgreSQL with no embedding
   extraction, since fixing a prediction isn't the same as teaching the model
   a new prompt.

Evaluate, Rescore, and Auto-label all run as background jobs with a progress
bar + ETA, since each is a full inference pass over a folder. Revisiting an
already-labeled image (either tab) shows its saved boxes automatically,
dimmed.

Save defaults to replacing an image's whole label file. Tick **update (keep
existing)** to add the drawn boxes to what's already saved instead of
replacing it — for going back and adding one more instance without redrawing
everything.

Toggle **Plain language** in the header to swap technical wording ("SAVPE
embedding", "prompt bank") for descriptions anyone can follow, without
changing what the tool does underneath.

`Bank.classes` is insertion-ordered, never alphabetized — a label file's
class column is an index into that list, so it stays fixed once assigned or
older files silently decode under the wrong class.

## Model selection

The **Model** picker (`ModelPicker.tsx`, shared by the session setup card and
the "Model" card in the main labeling screen — pick or change it from
wherever you're already looking, not just before opening a session) chooses
which YOLOE checkpoint a project teaches — from `yoloe-v8s-seg` (the oldest,
smallest generation) up through `yoloe-26x-seg` (the newest generation's
largest, most accurate size). The full catalog is in
[`backend/models.json`](backend/models.json) -- read by both the Go API and the
Python sidecar, so adding one is a single edit -- and served at
`GET /api/config`.

**The choice is locked in the moment the first box is saved.** Embeddings
from two different checkpoints don't share a vector space — mixing them into
one prompt bank would feed `set_classes()` vectors that don't mean the same
thing, silently or with a shape error. `Bank.lock_model()` enforces this the
same way class-index order is locked: the first embedding decides, and a
`model_id` that doesn't match what's already there gets a `409`, not a
silent switch. Once a project has a model, every picker becomes a read-only
chip — but it's not a dead end: a **"Switch model…"** button next to the
chip re-extracts every already-taught example under a new checkpoint and
swaps the lock (`Bank.reembed()`, `POST /api/reembed`, a background job
like Evaluate/Auto-label). Saved label files are never touched — only the
prompt bank's vectors and everything downstream of them (predict/evaluate/
auto-label from that point on) change, so cached confidence scores and any
measured F1 need re-checking afterward. Starting a new image folder is still
the way to keep two models' work side by side instead of overwriting one
with the other.

Each option carries a 🟢/🔴 dot: whether the checkpoint's weight file is
already on the server (`GET /api/config`'s `available` field, backed by
`internal/platform/models`'s `IsAvailable()` checking `MODELS_DIR` on disk) or still
needs an auto-download from GitHub the first time it's used — which can be
slow or fail outright with no route to `github.com`. `yoloe-11s-seg` (the
default), `yoloe-26s-seg`, and `yoloe-26x-seg` are pre-cached.

Outside Docker that's just the repo-local `models/` folder. In Docker,
`/models` inside the `vpe` container is the `models` *named volume*
(`docker-compose.yml`), not the repo folder of the same name on the host —
those are two different filesystems. (`api` mounts the same volume read-only,
purely to report which weights are present; `vpe` is the one that downloads
into it.) A **named volume that has never been mounted before seeds itself
from whatever's at that path in the image** (standard Docker behavior), so
`backend/inference/Dockerfile` bakes the same three `.pt` files into the image
at `/models/` — a fresh volume (first
`docker compose up`, or after `docker compose down -v`) comes up already
populated with no manual step. `docker cp`-ing a file into a *running*
container only patches that one volume instance and gets lost the next time
the volume is recreated — bake it into the image instead if it needs to
survive that. The rest of the catalog still downloads into the volume on
first use, same as before.

Bigger sizes are slower per image but generally more accurate; there's no
rule of thumb better than trying one on your own dataset's held-out test set
(see [Using CT-Flow](#using-ct-flow) step 3). On a 4 GB GPU the largest
checkpoints (`yoloe-26l-seg`, `yoloe-26x-seg`) may not fit — if a model fails
to load with an out-of-memory error, drop to a smaller size or run on CPU
(see [Quick start](#quick-start-docker)).

## Keyboard shortcuts

Opens with **`?`** in the app; inert while a text field or dialog has focus.

| Key | Action |
|---|---|
| `Enter` / `Ctrl`+`S` | Save (or Save review, in review mode) |
| `→` / `N` / `S` | Next image |
| `←` / `P` | Previous image |
| `Ctrl`+`Z` / `Ctrl`+`Shift`+`Z` | Undo / redo box edits |
| `1`–`9` | Set the active class |
| `Delete` / `Backspace` | Remove the selected box |
| `Esc` | Clear all drawn boxes on this image |
| `C` | Paste the clipboard's boxes (copied from another image) |
| `A` | Accept all predicted boxes in review mode |

## Multi-user & security (optional)

Off by default: with no `LABEL_TOOL_USERS` set, there is no login, and
`POST /api/upload` refuses to run in `vm` mode — a shared server that takes
files from anyone who knows the URL is worse than no upload at all. Turn
both on together:

```bash
docker compose run --rm --entrypoint /app/api api -hash-password alice 'their password'
# -> alice:pbkdf2$240000$...   put it in LABEL_TOOL_USERS (comma-separated)
python -c "import secrets; print(secrets.token_hex(32))"   # LABEL_TOOL_SECRET
```

Then every endpoint except `GET /api/config` and `/api/auth/*` needs a signed
session cookie, and every prompt-bank instance records the `labeled_by` who
taught it. Passwords are PBKDF2-HMAC-SHA256 (240k iterations) compared in
constant time — no user database, and the hash format is unchanged from before
the Go port, so an existing `LABEL_TOOL_USERS` value keeps working.

**There is no sign-in screen in the UI yet** — the header shows a permanent
"Not signed in" indicator, and signing in currently means calling
`POST /api/auth/login` directly. See [Known limitations](#known-limitations--roadmap).

## Configuration reference

| env | default | meaning |
|---|---|---|
| `DATA_DIR` | `../data` | host folder mounted at `/opt/mount/project` — datasets and outputs must live under it (default is the sibling `data/` folder shared with the original POC) |
| `WEB_PORT` | `3000` | port the UI is served on |
| `LABEL_TOOL_MODE` | `vm` in Docker | `vm` = confined to `LABEL_TOOL_VM_ROOT`; `local` = browse every drive |
| `LABEL_TOOL_VM_ROOT` | `/opt/mount/project` | the confinement root in `vm` mode |
| `MODELS_DIR` | `/models` in Docker | where YOLOE checkpoints are cached after auto-download — a named volume in Docker, a plain repo-local folder otherwise |
| `POSTGRES_PASSWORD` | *(none — required)* | password for the `db` service; `docker compose up` refuses to start without it |
| `DATABASE_URL` | set automatically in compose | where label/box storage lives (PostgreSQL, see [docs/DB_MIGRATION_PLAN.md](docs/DB_MIGRATION_PLAN.md)) — override to point at a different Postgres when running outside Docker |
| `LABEL_TOOL_USERS` | *(empty)* | `name:hash,name:hash` — empty means no login and no upload |
| `LABEL_TOOL_SECRET` | *(random per restart)* | signs the session cookie; unset = everyone signed out on every restart |
| `LABEL_TOOL_MAX_UPLOAD_MB` | `25` | per-file upload cap |
| `APP_UID` | `1000` | build arg — must own `DATA_DIR` on a Linux host, since the container doesn't run as root |
| `TORCH_INDEX_URL` | `.../whl/cu126` | build arg — the pip index PyTorch installs from; override to `.../whl/cpu` for a GPU-less build |

`local` mode only makes sense outside Docker, where the server runs on your
own PC and browsing the server means browsing your machine.

## API overview

Full request/response shapes: [`docs/API_REFERENCE.md`](docs/API_REFERENCE.md)
(Thai) — that document is the reference; there is no Swagger UI. Every endpoint
is under `/api`, called only through the Next.js proxy
(`app/api/[...path]/route.ts`) — never directly from the browser.

| Handler | Base path | Endpoints |
|---|---|---|
| `system.go` | `/api` | `GET /config`, `GET /browse`, `GET /image` |
| `pool.go` / `label.go` | `/api` | `POST /session`, `GET /boxes`, `POST /label`, `POST /predict`, `POST /relabel` |
| `project.go` | `/api` | `GET`/`POST`/`DELETE /history`, `GET`/`POST /events` |
| `testset.go` | `/api/testset` | `POST /import`, `POST /remove`, `POST /label` |
| `jobs.go` | `/api` | `GET /jobs/{id}`, `POST /score`, `POST /evaluate`, `POST /autolabel`, `POST /reembed` |
| `auth.go` | `/api/auth` | `GET /me`, `POST /login`, `POST /logout` |
| `upload.go` | `/api` | `POST /upload` |
| `export.go` | `/api` | `GET /export` |

Every one of these operates on a single `input_dir` a client sends — the
prompt bank, labels, and test-set manifest all live in a fixed `.ctflow/`
subfolder of it (see `backend/deps.py`), so there's no separate output or
test-set folder to pass. Conventions worth knowing before calling any of
these directly: errors are always `{"detail": "<message>"}`; any endpoint
taking a path from the browser (`input_dir`, an image path) is validated by
`deps.checked_path()` and returns `403` outside the allowed root; boxes are
always `{"cls": str, "box": [x1, y1, x2, y2]}` in source-image pixels, never
normalized.

## Local development (without Docker)

Three processes: PostgreSQL, the inference sidecar, and the API. Run everything
from the repo root so `backend` resolves as a package.

```bash
# 1. database (or point DATABASE_URL at one you already have)
docker compose up -d db

# 2. inference sidecar -- needs torch + ultralytics
pip install -r backend/inference/requirements.txt
uvicorn backend.inference.service:app --port 8001 --reload

# 3. API
export DATABASE_URL=postgresql://labeltool:<password>@localhost:5433/labeltool
cd backend && go run ./cmd/api      # :8000, VPE_URL defaults to 127.0.0.1:8001

cd frontend && npm run dev          # needs API_URL if not 127.0.0.1:8000
```

`go run` recompiles in a couple of seconds, so the API needs no reload watcher;
the sidecar keeps `--reload`, though note that a reload there drops every loaded
checkpoint and the next request pays the cold start again.

The sidecar uses whatever torch build is installed — install a
[CUDA build](https://pytorch.org/get-started/locally/) yourself if your GPU and
driver support it, or leave the CPU build; either works, `vpe.py` never
hardcodes a device.

```bash
# T-01 conf-threshold sweep -- still Python, needs data/conveyor_pvc
python -m backend.tools.experiment_conf 20
```

## Testing & CI

**`backend/tests/smoke_test.py` is the end-to-end check**, and it drives whatever is
listening on `SMOKE_BASE_URL` rather than importing the app — session, label,
predict (including a mid-process class-count change, the regression guard for a
real bug found once), evaluate, auto-label, review, upload, auth, history and
events, asserting against PostgreSQL and the files on disk. No mocks. That
indirection is what let the Go port be held to the same assertions the FastAPI
service passed, one command, no second suite to drift.

```bash
cd backend && go test ./...              # Go unit tests (store needs DATABASE_URL)

# the rest run from the repo root, so `backend` resolves as a package
pip install -r backend/tests/requirements.txt
SMOKE_BASE_URL=http://localhost:8000 python -m backend.tests.smoke_test
python -m backend.tests.bank_test              # prompt-bank unit checks (needs torch)
python -m backend.tests.gen_testdata --check   # golden vectors vs. tools/metrics.py
python -m backend.inference.models             # checkpoint catalog self-check
python -m backend.tools.metrics                # precision/recall/F1 self-check
python -m backend.tools.groundtruth            # file-based ground-truth self-check
```

`backend/tests/testdata/` holds cross-language golden vectors — pbkdf2 hashes, signed
cookies, F1 results, COCO/VOC/YOLO output — that Go's unit tests reproduce
exactly. Most were generated by Python that no longer exists, which is what
makes them the record of the behaviour the port had to match.

`backend/tests/parity.py` diffs two running backends endpoint by endpoint, floats to
1e-9. It was the gate for every step of the port and is still the tool for
"did this change anything a client can see":

```bash
python -m backend.tests.parity --a http://localhost:8100 --b http://localhost:8000 \
    --pool-a /data/_parity_a --pool-b /data/_parity_b
```

`.github/workflows/backend.yml` runs three jobs on every push/PR touching
`backend/**` — which is now the whole backend, Go included: `go` (vet, gofmt, tests against a real PostgreSQL), `python` (the
sidecar's self-checks), and `smoke`, which starts a real API and a real sidecar
and drives them over HTTP, caching the 28 MB weight between runs.

## Datasets & measured accuracy

All datasets live outside this repo (see `../data/README.md`) and mount at
`/opt/mount/project` in the container. `/opt/mount/project/conveyor_pvc` — PVC fittings from
[Roboflow's conveyor-belt v3](https://universe.roboflow.com/onkar/conveyor-belt)
(CC BY 4.0) — is the one with a documented accuracy ceiling worth knowing
before you rely on this tool for a similar dataset:

| conf threshold | `good_part` F1 | `defect` F1 | `defect` recall |
|---|---|---|---|
| 0.25 (old default) | 0.82 | 0.00 | 0.00 |
| 0.10 | 0.77 | 0.06 | 0.04 |
| 0.05 | 0.59 | 0.25 | 0.26 |
| **per-class (`conf_by_class`)** | **0.82** | **0.25** | — |

`defect` (small chips/scratches) needs a much lower threshold than
`good_part` to show up at all — median box size is 2.09% of the image for
`defect` vs. 43.45% for `good_part`, a ~20x gap. `conf_by_class` gets both
classes their own threshold in one evaluation pass instead of trading one off
against the other. Full methodology and every intermediate number:
[`docs/EXPERIMENT_T01_CONF.md`](docs/EXPERIMENT_T01_CONF.md).

`/opt/mount/project/iron_ore` (235 frames of rock/ore on a moving belt) is the dataset
that matches the real production use case; label ~15 images into `test/`
first to get a readiness number for it.

## Known limitations & roadmap

Full gap analysis: [`docs/NEXT_STEPS.md`](docs/NEXT_STEPS.md) and
[`docs/REQUIREMENTS_STAKEHOLDER_ANALYSIS.md`](docs/REQUIREMENTS_STAKEHOLDER_ANALYSIS.md).
The headline items:

- **No sign-in / upload UI.** Both are fully built and tested on the backend
  (`/api/auth/*`, `POST /api/upload`); nothing in the frontend calls them
  yet. Building the two screens is the next-highest-leverage frontend work.
- **`defect`-class recall is still low even with `conf_by_class`** (0.25 F1).
  The box-size gap above points at cropping around each detection before
  running SAVPE on it as the next lever — unblocked and prioritized ahead of
  further threshold tuning.
- **Duplicate/near-duplicate detection uses an 8×8 thumbnail hash**, not true
  embedding distance. Cheap and has been good enough in practice; swapping
  in real embedding distance would roughly double rescore latency, so it's
  deferred until it's a measured problem.
- **GPU/non-root Docker build succeeds; runtime is unverified.**
  `docker compose build api` completed successfully with the CUDA torch
  wheel and the non-root user setup, but Docker Desktop's daemon stopped
  responding (`500` on every API call) right after — `docker compose up` and
  an actual `torch.cuda.is_available()` check inside the running container
  are still outstanding. Restart Docker Desktop and re-run to finish
  verifying.

## Documentation index

Deeper design and requirements docs live in `docs/` (Thai, written for
whoever picks this project up next):

| Doc | What's in it |
|---|---|
| [`PRODUCT_OVERVIEW.md`](docs/PRODUCT_OVERVIEW.md) | What the tool does and doesn't do, in plain terms |
| [`ARCHITECTURE.md`](docs/ARCHITECTURE.md) | Tech stack + system design |
| [`API_REFERENCE.md`](docs/API_REFERENCE.md) | Every endpoint, request/response shapes, conventions |
| [`REQUIREMENTS_STAKEHOLDER_ANALYSIS.md`](docs/REQUIREMENTS_STAKEHOLDER_ANALYSIS.md) | Requirement-by-requirement status, the source of truth for "is X done" |
| [`PROJECT_STATUS.md`](docs/PROJECT_STATUS.md) | Test coverage + current overall status |
| [`EXPERIMENT_T01_CONF.md`](docs/EXPERIMENT_T01_CONF.md) | The conf-threshold experiment behind the accuracy table above |
| [`REFACTOR_PLAN.md`](docs/REFACTOR_PLAN.md) | The Python→Go port: what moved, what stayed Python and why, and what it cost |
| [`NEXT_STEPS.md`](docs/NEXT_STEPS.md) | Prioritized gap list for the next round of work |
| [`GLOSSARY.md`](docs/GLOSSARY.md) | Terminology (SAVPE, prompt bank, etc.) in the order you'll meet it |

## Troubleshooting

`docker compose build` failing with `CERTIFICATE_VERIFY_FAILED`? Something is
intercepting TLS during the build — a corporate proxy, or antivirus HTTPS
scanning (AVG/Avast/Kaspersky all do this). Drop the intercepting root CA
into `certs/`; see [`certs/README.md`](certs/README.md) for how to export it
from the Windows certificate store.

---

*Internal Connected Tech tool. No open-source license — do not redistribute
outside the organization.*
