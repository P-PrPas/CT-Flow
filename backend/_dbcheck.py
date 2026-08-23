"""Read-only PostgreSQL assertions for _smoke_test.py.

Deliberately does NOT import services/annotations_db.py: the smoke test has to
keep working after that module is ported to Go and deleted (see
docs/REFACTOR_PLAN.md phase 3). Duplicating five SELECTs here is what lets the
harness outlive the implementation it checks -- a test that imports the code
under test can only ever verify Python.

Every function returns an empty result for a project that does not exist yet,
rather than creating one the way annotations_db's get_or_create_project does --
a checker must never be the thing that puts a row in the table.
"""
import os
from pathlib import Path

import psycopg2

DATABASE_URL = os.getenv(
    "DATABASE_URL", "postgresql://labeltool:labeltool@localhost:5432/labeltool"
)
SCHEMA_PATH = Path(__file__).resolve().parent / "schema.sql"


def _query(sql: str, args: tuple | None = None):
    """args=None skips psycopg2's parameter interpolation entirely, so a
    statement containing a literal % (schema.sql today has none, but DDL is
    exactly where one shows up) can't be mistaken for a placeholder."""
    conn = psycopg2.connect(DATABASE_URL)
    try:
        with conn, conn.cursor() as cur:
            cur.execute(sql) if args is None else cur.execute(sql, args)
            return cur.fetchall() if cur.description else None
    finally:
        conn.close()


def init_schema() -> None:
    """schema.sql is idempotent (CREATE TABLE IF NOT EXISTS), so running it
    from the harness is safe even when the server already did it at startup --
    and it's what lets the smoke test set up its own fixture without importing
    services/db.py, which Go replaces."""
    _query(SCHEMA_PATH.read_text(encoding="utf-8"))


def _project_id(input_dir: str) -> int | None:
    rows = _query("SELECT id FROM projects WHERE input_dir=%s", (input_dir,))
    return rows[0][0] if rows else None


def get_classes(input_dir: str, kind: str) -> list[str]:
    """idx order, not alphabetical -- the append-only contract classes.txt had."""
    pid = _project_id(input_dir)
    if pid is None:
        return []
    return [r[0] for r in _query(
        "SELECT name FROM classes WHERE project_id=%s AND kind=%s ORDER BY idx", (pid, kind)
    )]


def read_boxes(input_dir: str, kind: str, image_path: str) -> list[dict]:
    pid = _project_id(input_dir)
    if pid is None:
        return []
    rows = _query(
        "SELECT c.name, a.x1, a.y1, a.x2, a.y2 FROM annotations a "
        "JOIN images i ON i.id = a.image_id "
        "JOIN classes c ON c.id = a.class_id "
        "WHERE i.project_id=%s AND i.kind=%s AND i.path=%s ORDER BY a.id",
        (pid, kind, image_path),
    )
    return [{"cls": r[0], "box": [r[1], r[2], r[3], r[4]]} for r in rows]


def list_by_status(input_dir: str, kind: str) -> dict[str, list[str]]:
    pid = _project_id(input_dir)
    rows = [] if pid is None else _query(
        "SELECT path, status FROM images WHERE project_id=%s AND kind=%s AND status != 'unlabeled'",
        (pid, kind),
    )
    return {
        "labeled": sorted(p for p, s in rows if s == "labeled"),
        "auto": sorted(p for p, s in rows if s == "auto"),
    }


def delete_project(input_dir: str) -> None:
    """Test fixture reset -- classes/images/annotations cascade."""
    _query("DELETE FROM projects WHERE input_dir=%s", (input_dir,))


def demo():
    missing = "/nonexistent/project/for/selfcheck"
    assert _project_id(missing) is None
    assert get_classes(missing, "pool") == []
    assert read_boxes(missing, "pool", "x.jpg") == []
    assert list_by_status(missing, "pool") == {"labeled": [], "auto": []}
    # a lookup must not have created the row it failed to find
    assert _project_id(missing) is None
    print("dbcheck self-check OK")


if __name__ == "__main__":
    demo()
