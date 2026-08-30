"""One-off recovery: rebuild a project's PostgreSQL rows from its `.ctflow`
prompt bank, for the half-wiped state FR-51 warns about -- `docker compose
down -v` ran, the dataset's `.ctflow/` did not, so the bank still remembers
what it was taught but `classes`/`images`/`annotations` are empty.

NOT an endpoint and NOT wired into the UI on purpose (see
docs/PHASE2_WORKSPACE.md #8): the normal fix is `pg_dump` + restore of both
halves together, and code that writes the append-only `classes.idx` should
run by hand, once, with someone watching -- not on a button.

    DATABASE_URL=postgresql://labeltool:PW@localhost:5433/labeltool \
        python -m backend.tools.import_ctflow --input-dir /opt/mount/project/mydataset --dry-run

    # looks right? drop --dry-run:
    DATABASE_URL=... python -m backend.tools.import_ctflow --input-dir /opt/mount/project/mydataset

    python -m backend.tools.import_ctflow --self-check    # no DB needed

What it restores (kind='pool' only):
  - classes, in `metadata.json` insertion order, used as the idx a label's
    class column means (invariant #1). This is the part that has to be exact,
    and it rests on an ASSUMPTION this tool cannot check: the idx that
    matters is the order of `embeddings.pt` (`Bank.classes` is
    `list(self.embeddings.keys())`), and metadata.json is a second file.
    `Bank.add()` appends to both under one lock so they agree in normal
    operation -- but a crash between the two halves of `_save()` is exactly
    the kind of half-written state this tool exists to clean up after, and
    reading embeddings.pt to compare would mean importing torch here.
    So: the run prints the class list it is about to write. Compare it,
    in order, against what the app reports for that bank (the `classes`
    array from POST /api/session) before answering the prompt.
  - one images row per distinct source_image, status='labeled'.
  - one annotations row per bank instance (bbox + labeled_by + added_at).

What it CANNOT restore -- the bank never held it:
  - the test set. pool and testset are separate index spaces and a test
    image never teaches the bank, so F1 ground truth is gone. Rebuild it by
    hand.
  - images that were only ever autolabeled (status='auto'): DB-only.
  - boxes beyond the first of a given class in a given teach action:
    /vpe/teach records `cls_boxes[0]` per class, averages the rest into the
    embedding without storing them (same limit reembed_stream has).
  - corrections: bank.add() is append-only and /api/relabel never touches
    the bank, so a box taught then fixed is still in the bank as taught --
    it comes back here. Eyeball the result.
"""
import argparse
import json
import os
import sys
from datetime import datetime, timezone
from pathlib import Path

DATABASE_URL = os.getenv(
    "DATABASE_URL", "postgresql://labeltool:labeltool@localhost:5432/labeltool"
)


def _plan(meta: dict):
    """Pure: metadata.json dict -> (classes_in_order, images_sorted, rows).

    rows: (source_image, class_name, [x1,y1,x2,y2], labeled_by|None, added_at|None)
    Split out from the DB work so --self-check can exercise the ordering and
    flattening -- the only part with a way to be silently wrong.
    """
    instances: dict[str, list[dict]] = meta.get("instances", {})
    classes = list(instances.keys())  # insertion order == embeddings.pt order
    rows = []
    images = set()
    for name in classes:
        for inst in instances[name]:
            src = inst["source_image"]
            bbox = [float(v) for v in inst["bbox"]]
            if len(bbox) != 4:
                raise ValueError(f"instance of {name!r} has bbox {inst['bbox']!r}, want 4 numbers")
            images.add(src)
            rows.append((src, name, bbox, inst.get("labeled_by"), inst.get("added_at")))
    return classes, sorted(images), rows


def _load_meta(input_dir: Path) -> dict:
    p = input_dir / ".ctflow" / "_bank" / "metadata.json"
    if not p.exists():
        sys.exit(f"no bank at {p} -- nothing to import")
    meta = json.loads(p.read_text(encoding="utf-8"))
    if not meta.get("instances"):
        sys.exit(f"{p} has no taught instances -- nothing to import")
    return meta


def run(input_dir: Path, name: str | None, owner: str | None, dry_run: bool, assume_yes: bool):
    import psycopg2  # deferred: --self-check needs neither the driver nor a DB

    meta = _load_meta(input_dir)
    classes, images, rows = _plan(meta)
    model = meta.get("model")
    input_dir_s = str(input_dir)

    print(f"bank:     {input_dir_s}/.ctflow/_bank")
    print(f"model:    {model or '(unlocked -- unexpected for a bank with instances)'}")
    print(f"restores: {len(classes)} classes, {len(images)} images, {len(rows)} boxes (pool)")
    print(f"classes:  {classes}")

    conn = psycopg2.connect(DATABASE_URL)
    try:
        cur = conn.cursor()

        cur.execute("SELECT id FROM projects WHERE input_dir=%s", (input_dir_s,))
        got = cur.fetchone()
        if got:
            pid = got[0]
            cur.execute(
                "SELECT "
                " (SELECT count(*) FROM classes WHERE project_id=%s AND kind='pool'),"
                " (SELECT count(*) FROM images  WHERE project_id=%s AND kind='pool')",
                (pid, pid),
            )
            nc, ni = cur.fetchone()
            if nc or ni:
                sys.exit(
                    f"project {pid} already has {nc} pool classes and {ni} pool images -- "
                    "this importer only fills an empty (bank_orphaned) project. "
                    "Restore from a pg_dump instead, or DELETE the project first if you mean to."
                )
            print(f"project:  {pid} (exists, empty)")
        else:
            pname = name or input_dir.name
            cur.execute(
                "INSERT INTO projects (input_dir, name, owner_oid, task_type) "
                "VALUES (%s, %s, %s, 'detection') RETURNING id",
                (input_dir_s, pname, owner),
            )
            pid = cur.fetchone()[0]
            print(f"project:  {pid} (created, name={pname!r}, owner={owner or 'NULL'})")

        # classes -- idx is the enumerate position, which is metadata.json /
        # embeddings.pt insertion order. Nothing else may have written a pool
        # class (checked above), so idx starts at 0.
        class_id: dict[str, int] = {}
        for idx, cname in enumerate(classes):
            cur.execute(
                "INSERT INTO classes (project_id, kind, idx, name) VALUES (%s, 'pool', %s, %s) RETURNING id",
                (pid, idx, cname),
            )
            class_id[cname] = cur.fetchone()[0]

        image_id: dict[str, int] = {}
        for path in images:
            cur.execute(
                "INSERT INTO images (project_id, kind, path, status) VALUES (%s, 'pool', %s, 'labeled') "
                "ON CONFLICT (project_id, kind, path) DO UPDATE SET status='labeled' RETURNING id",
                (pid, path),
            )
            image_id[path] = cur.fetchone()[0]

        for src, cname, bbox, labeled_by, added_at in rows:
            created_at = (
                datetime.fromtimestamp(added_at, tz=timezone.utc) if added_at else None
            )
            cur.execute(
                "INSERT INTO annotations (image_id, class_id, x1, y1, x2, y2, created_by, created_at) "
                "VALUES (%s, %s, %s, %s, %s, %s, %s, COALESCE(%s, now()))",
                (image_id[src], class_id[cname], bbox[0], bbox[1], bbox[2], bbox[3],
                 labeled_by, created_at),
            )

        if dry_run:
            conn.rollback()
            print("\ndry run -- rolled back, database unchanged")
            return
        if not assume_yes:
            if input("\ncommit these rows? [y/N] ").strip().lower() != "y":
                conn.rollback()
                sys.exit("aborted")
        conn.commit()
        print("\ncommitted.")
        print("next: rebuild the test set by hand -- it was not in the bank and F1 needs it.")
    finally:
        conn.close()


def demo():
    meta = {
        "model": "yoloe-v8l-seg",
        "instances": {
            "cube": [
                {"source_image": "/d/b.jpg", "bbox": [1, 2, 3, 4], "added_at": 1_700_000_000.0, "labeled_by": "sub-1"},
                {"source_image": "/d/a.jpg", "bbox": [5, 6, 7, 8], "added_at": 1_700_000_100.0, "labeled_by": None},
            ],
            "sphere": [
                {"source_image": "/d/a.jpg", "bbox": [9, 10, 11, 12], "added_at": None, "labeled_by": "sub-2"},
            ],
        },
    }
    classes, images, rows = _plan(meta)
    assert classes == ["cube", "sphere"], classes            # insertion order, not sorted
    assert images == ["/d/a.jpg", "/d/b.jpg"], images        # deduped + sorted
    assert len(rows) == 3
    assert rows[0] == ("/d/b.jpg", "cube", [1.0, 2.0, 3.0, 4.0], "sub-1", 1_700_000_000.0)
    assert rows[2][3] == "sub-2" and rows[2][4] is None

    try:
        _plan({"instances": {"x": [{"source_image": "/d/x.jpg", "bbox": [1, 2, 3]}]}})
        assert False, "expected a bbox-arity error"
    except ValueError:
        pass
    print("import_ctflow self-check OK")


def main():
    ap = argparse.ArgumentParser(description="Rebuild a project's DB rows from its .ctflow bank.")
    ap.add_argument("--input-dir", type=Path, help="the dataset folder (the one that holds .ctflow/)")
    ap.add_argument("--name", help="project name if it must be created (default: folder name)")
    ap.add_argument("--owner", help="owner_oid if the project must be created (default: NULL)")
    ap.add_argument("--dry-run", action="store_true", help="do everything, then roll back")
    ap.add_argument("--yes", action="store_true", help="skip the confirmation prompt")
    ap.add_argument("--self-check", action="store_true", help="run the offline self-check and exit")
    args = ap.parse_args()

    if args.self_check:
        demo()
        return
    if not args.input_dir:
        ap.error("--input-dir is required (or use --self-check)")
    run(args.input_dir.resolve(), args.name, args.owner, args.dry_run, args.yes)


if __name__ == "__main__":
    main()
