"""PostgreSQL connection + schema bootstrap for annotation storage (T-21).
Plain psycopg2, no ORM -- matches the rest of this backend (services/auth.py
is stdlib pbkdf2, services/bank.py is torch.save, nothing here uses one
either). See docs/DB_MIGRATION_PLAN.md for why label storage moved here
while the prompt bank (embeddings.pt) stayed a file.
"""
import os
from contextlib import contextmanager
from pathlib import Path

import psycopg2

DATABASE_URL = os.getenv(
    "DATABASE_URL", "postgresql://labeltool:labeltool@localhost:5432/labeltool"
)
SCHEMA_PATH = Path(__file__).resolve().parent.parent / "schema.sql"


@contextmanager
def connect():
    """One connection, one transaction: commits on a clean exit, rolls back
    on exception. Every write in annotations_db.py wraps its statements in a
    single `with connect() as conn:` so a mid-request failure can't leave a
    class registered without its annotation, or an image row without the
    boxes it was supposed to get."""
    conn = psycopg2.connect(DATABASE_URL)
    try:
        yield conn
        conn.commit()
    except Exception:
        conn.rollback()
        raise
    finally:
        conn.close()


def init_schema():
    """Idempotent (CREATE TABLE IF NOT EXISTS) -- safe to call on every
    process start (see app.py's startup)."""
    with connect() as conn, conn.cursor() as cur:
        cur.execute(SCHEMA_PATH.read_text(encoding="utf-8"))


def demo():
    init_schema()
    with connect() as conn, conn.cursor() as cur:
        cur.execute("SELECT 1")
        assert cur.fetchone() == (1,)
    print("db self-check OK")


if __name__ == "__main__":
    demo()
