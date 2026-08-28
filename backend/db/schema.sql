-- T-21: label/box storage. Everything the prompt bank itself needs
-- (embeddings.pt, metadata.json's instances/model) stays file-based --
-- see docs/history/DB_MIGRATION_PLAN.md for why the split. This is only the part
-- that used to be labels/*.txt + classes.txt + testset.json.
--
-- ponytail: no migration framework (Alembic etc) -- this schema is small
-- and changes rarely; add one if that stops being true. Idempotent
-- (IF NOT EXISTS) so services/db.py can run it on every process start.

-- One row per dataset folder. `input_dir` stays the identity every endpoint and
-- every query below uses; `id` exists only so the UI has something to put in a
-- URL that is not a server path (docs/PHASE2_WORKSPACE.md #2, decision 6).
--
-- No ALTER TABLE for the Phase 2 columns, and no migration framework. Adding
-- them alongside the CREATE would leave a database created fresh and a database
-- upgraded in place with two different definitions of `name` (NOT NULL vs
-- nullable) from one file, which is a worse problem than the reset it avoids.
-- Wiping is the migration -- see docs/PHASE2_WORKSPACE.md #8 -- and
-- Store.InitSchema fails loudly at boot if these columns are missing, rather
-- than letting a stale database break at a user's first request.
CREATE TABLE IF NOT EXISTS projects (
    id          BIGSERIAL PRIMARY KEY,
    input_dir   TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    -- users.oid of whoever created it. Nullable: a project can outlive the
    -- account that made it, and nothing here should refuse to list one.
    owner_oid   TEXT,
    -- Which labeling module owns this project. One value today, and anything
    -- else is rejected at the handler. It is here so a second module is a
    -- sibling branch rather than a rewrite -- not as the seam of an
    -- abstraction, which is a thing to discover from module two, not invent
    -- before module one has users.
    task_type   TEXT NOT NULL DEFAULT 'detection',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Touched by every write path through requireProject, so "last worked on"
    -- costs no extra statement.
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Append-only, stable idx -- the DB-native replacement for classes.txt's
-- "line N = index N, never reordered". pool and testset are separate index
-- spaces (see docs/history/DB_MIGRATION_PLAN.md #2), hence `kind`.
CREATE TABLE IF NOT EXISTS classes (
    id          BIGSERIAL PRIMARY KEY,
    project_id  BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL CHECK (kind IN ('pool', 'testset')),
    idx         INT NOT NULL,
    name        TEXT NOT NULL,
    UNIQUE (project_id, kind, idx),
    UNIQUE (project_id, kind, name)
);

-- One row per (project, kind, path). A pool image and its testset
-- counterpart are two different rows sharing the same `path` -- no file
-- copy, same convention the old testset.json manifest used.
CREATE TABLE IF NOT EXISTS images (
    id          BIGSERIAL PRIMARY KEY,
    project_id  BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL CHECK (kind IN ('pool', 'testset')),
    path        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'unlabeled' CHECK (status IN ('unlabeled', 'labeled', 'auto')),
    UNIQUE (project_id, kind, path)
);

-- One row per box -- replaces one line of labels/<stem>.txt. Pixel coords,
-- same convention the Box model in API_REFERENCE.md always used (no more
-- normalize-on-write / denormalize-on-read).
CREATE TABLE IF NOT EXISTS annotations (
    id          BIGSERIAL PRIMARY KEY,
    image_id    BIGINT NOT NULL REFERENCES images(id) ON DELETE CASCADE,
    class_id    BIGINT NOT NULL REFERENCES classes(id) ON DELETE CASCADE,
    x1 REAL NOT NULL, y1 REAL NOT NULL, x2 REAL NOT NULL, y2 REAL NOT NULL,
    created_by  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS annotations_image_id_idx ON annotations (image_id);

-- Who a `sub` is. OIDC attribution (FR-31) writes the provider's subject into
-- annotations.created_by, events.jsonl and the prompt bank's labeled_by,
-- because that is the one claim that survives a rename -- and it is also
-- unreadable, so on its own it turns "who taught this prompt" into a UUID
-- nobody can answer. This table is the answer, written on every login.
--
-- Shape follows corpus-core's users table (`oid` is its name for the `sub`
-- claim), minus what CT-Flow has no screen for: no firstname/lastname split,
-- no picture_url, no is_admin, no metadata blob. Add them when something
-- actually renders them.
--
-- Deliberately *not* copied from corpus-core: UNIQUE on username and email.
-- Only `oid` identifies a person. A provider that reissues a
-- preferred_username to a new subject -- an account deleted and recreated --
-- would turn a legitimate login into a constraint violation, and the rejected
-- one would be the current employee.
--
-- annotations.created_by stays TEXT rather than becoming a FK: legacy
-- LABEL_TOOL_USERS logins have no row here, and the prompt bank is a JSON file
-- written by the Python sidecar, which cannot join anything.
CREATE TABLE IF NOT EXISTS users (
    id          BIGSERIAL PRIMARY KEY,
    oid         TEXT NOT NULL UNIQUE,
    username    TEXT NOT NULL,
    email       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
