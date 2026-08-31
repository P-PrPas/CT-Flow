# CT-Flow — working notes for coding agents

Human-in-the-loop visual-prompt labeling tool for YOLOE. Read this, then
[`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md), then the plan for whatever you
are working on. `README.md` is the user-facing entry point.

## Where things are

```
backend/cmd/api           the Go API binary
backend/internal/         transport/httpapi · core · infra · platform
backend/inference/        the Python sidecar: everything that needs torch
backend/db/schema.sql     applied at boot, idempotent, no migration framework
backend/tests/            smoke_test.py, golden vectors, parity harness
frontend/app/             Next.js 15 App Router, all client components
frontend/app/modules/     one folder per labeling module; the shell must not
                          import from here -- CI enforces it (T-30)
docs/                     active docs -- must be true right now
docs/history/             records of completed work -- context, not status
```

## Invariants — breaking one of these corrupts data silently

1. **Class indexes are append-only.** Never reorder, never delete, never
   reuse. A label references its class by position. This holds in
   `classes.idx` (PostgreSQL) *and* in `Bank.classes` (insertion order of
   `embeddings.pt`), and the two must stay in the same order.
2. **Pool and testset are separate index spaces.** Same project, different
   `kind`, independent numbering.
3. **A test-set image can never teach the prompt bank.** `POST /api/label`
   and `/api/relabel` return `400` for it. The F1 number is worthless
   otherwise.
4. **One prompt bank is locked to one checkpoint.** Embeddings from two
   YOLOE checkpoints do not share a vector space. `Bank.lock_model()` sets it
   on the first embedding and returns `409` for any other `model_id`
   afterwards. Only `POST /api/reembed` changes it, atomically.
5. **The Python sidecar owns `_bank/embeddings.pt` and `_bank/metadata.json`
   alone.** Go never touches those two files — `lock_model()`/`reembed()`
   commit under one file lock and a second writer breaks them.
   `eval_history.json` and `events.jsonl` live in the same folder but belong
   to Go.
6. **`arm()` mutates process-global model state.** It is held under an
   `RLock` per `model_id` for the whole batch (`inference/vpe.py::armed`).
   Two projects on the same checkpoint running concurrently without it do not
   crash — they answer with the wrong class names.
7. **No package under `internal/` imports `net/http` except
   `transport/httpapi`.** Handlers never touch the database directly.
8. **Every path from the browser goes through `checkedPath()`** before it
   reaches the disk. It resolves symlinks and compares path components, not
   string prefixes. The Python sidecar confines separately against the same
   `LABEL_TOOL_VM_ROOT` — both processes must be given it, or the boundary
   holds only in whichever process a path happens to reach first.
9. **A subject is not a name.** Attribution stores the OIDC `sub` everywhere
   (`annotations.created_by`, the bank's `labeled_by`, `projects.owner_oid`)
   because it survives a rename. `store.UserNames` is the only place that turns
   one back into a person: compare on `oid`, display `username`, and never put
   a raw subject where a person is meant to be read. Under a local login the
   two are the same string, so a confusion between them passes dev and CI and
   fails only on a real OIDC deployment.
10. **Writes require a project that already exists** (`store.ErrNoProject`,
    surfaced as one 404 in `Handle`). Only `EnsureProject` creates one, from
    `POST /api/projects` or `POST /api/session` — so every project has a name
    and an owner.
11. **Shared frontend code must not import from `app/modules/`.** Enforced by
    `.github/workflows/frontend.yml`, not by comment. If the home page needs to
    know what a prompt bank is, the seam is in the wrong place.

## Testing

`backend/tests/smoke_test.py` **is the contract.** It drives whatever is
listening on `SMOKE_BASE_URL` over HTTP — no imports, no mocks. Any endpoint
or behaviour change needs an assertion there. Do not start a second suite.

```bash
cd backend && go test ./...                       # needs DATABASE_URL
SMOKE_BASE_URL=http://localhost:8000 python -m backend.tests.smoke_test
python -m backend.tests.gen_testdata --check      # cross-language goldens
```

`backend/tests/testdata/` holds golden vectors Go must reproduce byte for
byte. If one fails, the code is wrong, not the vector.

`.github/workflows/frontend.yml` runs on any change under `frontend/`: the
module boundary, then `tsc --noEmit`, then `next build`. There are no frontend
tests and inventing some to justify a workflow is the wrong order -- what it
catches is broken imports, broken types and a broken build.

```bash
cd frontend && npx tsc --noEmit && npm run build
```

## House style

- **Ladder before code:** does it need to exist → stdlib → native platform
  feature → dependency already installed → one line → the minimum that works.
  Stop at the first rung that holds.
- No new dependencies for what a few lines do. The Go API is stdlib
  `net/http`; the frontend has no UI or state library.
- Mark deliberate shortcuts with a `ponytail:` comment naming the ceiling and
  the upgrade path (`// ponytail: in-memory, move to Redis with the job
  tracker`). Existing ones are a ledger, not litter.
- Deletion beats addition. An abstraction with one implementation is not a
  design, it is a guess.
- Match the surrounding code's comment density and naming. Comments explain
  *why*, since *what* is already in the code.

## Known ceilings (documented, not bugs to fix on sight)

- Job tracker and image claims are in-memory: one API process, no TTL
  persistence, no horizontal scale (NFR-06). Two replicas would not see each
  other's claims, which degrades to "two people, one image" rather than
  breaking.
- Models are cached per `model_id` per process with no VRAM eviction.
- Duplicate detection uses an 8×8 thumbnail hash, not embedding distance.
- The app serves no HTTPS of its own; that is a reverse proxy's job.
- The label list a project shows is the bank's classes plus a *plan* — names
  set up in the Labels dialog but not yet drawn — and a colour override per
  name. Both live in `localStorage` per `input_dir`, because a class is only
  created as a side effect of saving a box and there is no endpoint for either.
  Two people on one project can therefore see the same class in two colours.

## Conventions

- Errors are always `{"detail": "<message>"}`. Message text is asserted in
  tests and shown in the UI — do not reword casually.
- Boxes are always `{"cls": str, "box": [x1, y1, x2, y2]}` in source-image
  pixels, never normalized.
- Background jobs return `{"job_id", "total"}` immediately; the client polls
  `GET /api/jobs/{id}`. Jobs run on `context.Background()`, never the
  request's context.
- Commits: no `Co-Authored-By` trailer.
