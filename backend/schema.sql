-- T-21: label/box storage. Everything the prompt bank itself needs
-- (embeddings.pt, metadata.json's instances/model) stays file-based --
-- see docs/DB_MIGRATION_PLAN.md for why the split. This is only the part
-- that used to be labels/*.txt + classes.txt + testset.json.
--
-- ponytail: no migration framework (Alembic etc) -- this schema is small
-- and changes rarely; add one if that stops being true. Idempotent
-- (IF NOT EXISTS) so services/db.py can run it on every process start.

CREATE TABLE IF NOT EXISTS projects (
    id          BIGSERIAL PRIMARY KEY,
    input_dir   TEXT NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Append-only, stable idx -- the DB-native replacement for classes.txt's
-- "line N = index N, never reordered". pool and testset are separate index
-- spaces (see docs/DB_MIGRATION_PLAN.md #2), hence `kind`.
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
