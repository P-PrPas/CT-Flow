"""Label/box storage backed by PostgreSQL (T-21/T-22) -- the DB-native
replacement for labels/*.txt + classes.txt + testset.json. Prompt-bank
embeddings (_bank/embeddings.pt, metadata.json's instances/model) are a
separate concern and still live in a file -- see bank.py and
docs/DB_MIGRATION_PLAN.md for why the split.

Two `kind`s share one schema: 'pool' (the working set, taught to the model)
and 'testset' (held-out ground truth, never touches the bank) -- each has
its own class-index space, so a class named the same in both gets two
different rows and two different `idx` values. Nothing here enforces the
"never teach a test-set image" rule -- routers/pool.py still checks
is_test() before calling write_boxes(), same as it always did.
"""
from pathlib import Path

from . import db


def get_or_create_project(input_dir: str) -> int:
    with db.connect() as conn, conn.cursor() as cur:
        cur.execute(
            "INSERT INTO projects (input_dir) VALUES (%s) "
            "ON CONFLICT (input_dir) DO UPDATE SET input_dir = EXCLUDED.input_dir "
            "RETURNING id",
            (input_dir,),
        )
        return cur.fetchone()[0]


def _get_or_create_class(cur, project_id: int, kind: str, name: str) -> int:
    """Append-only, race-safe: locks the project row before computing the
    next `idx`, so two concurrent saves teaching two different new class
    names can't land on the same index (see docs/DB_MIGRATION_PLAN.md #4.1)
    -- the DB-native replacement for the filelock bank.py uses for the same
    purpose. The plain SELECT first avoids taking the lock at all on the
    (overwhelmingly common) case of an already-known class."""
    cur.execute(
        "SELECT id FROM classes WHERE project_id=%s AND kind=%s AND name=%s",
        (project_id, kind, name),
    )
    row = cur.fetchone()
    if row:
        return row[0]
    cur.execute("SELECT id FROM projects WHERE id=%s FOR UPDATE", (project_id,))
    cur.execute(
        "SELECT id FROM classes WHERE project_id=%s AND kind=%s AND name=%s",
        (project_id, kind, name),
    )
    row = cur.fetchone()
    if row:
        return row[0]
    cur.execute(
        "SELECT COALESCE(MAX(idx), -1) + 1 FROM classes WHERE project_id=%s AND kind=%s",
        (project_id, kind),
    )
    idx = cur.fetchone()[0]
    cur.execute(
        "INSERT INTO classes (project_id, kind, idx, name) VALUES (%s, %s, %s, %s) RETURNING id",
        (project_id, kind, idx, name),
    )
    return cur.fetchone()[0]


def get_or_create_class(input_dir: str, kind: str, name: str) -> int:
    project_id = get_or_create_project(input_dir)
    with db.connect() as conn, conn.cursor() as cur:
        return _get_or_create_class(cur, project_id, kind, name)


def get_classes(input_dir: str, kind: str) -> list[str]:
    """`idx` order, not alphabetical -- append-only, same contract the old
    classes.txt files had."""
    project_id = get_or_create_project(input_dir)
    with db.connect() as conn, conn.cursor() as cur:
        cur.execute(
            "SELECT name FROM classes WHERE project_id=%s AND kind=%s ORDER BY idx",
            (project_id, kind),
        )
        return [r[0] for r in cur.fetchall()]


def _get_or_create_image(cur, project_id: int, kind: str, path: str) -> int:
    cur.execute(
        "INSERT INTO images (project_id, kind, path) VALUES (%s, %s, %s) "
        "ON CONFLICT (project_id, kind, path) DO UPDATE SET path = EXCLUDED.path "
        "RETURNING id",
        (project_id, kind, path),
    )
    return cur.fetchone()[0]


def read_boxes(input_dir: str, kind: str, image_path: str) -> list[dict]:
    """[{"cls": name, "box": [x1,y1,x2,y2]}] in pixel coords, or [] if this
    image has no annotations yet."""
    project_id = get_or_create_project(input_dir)
    with db.connect() as conn, conn.cursor() as cur:
        cur.execute(
            "SELECT c.name, a.x1, a.y1, a.x2, a.y2 FROM annotations a "
            "JOIN images i ON i.id = a.image_id "
            "JOIN classes c ON c.id = a.class_id "
            "WHERE i.project_id=%s AND i.kind=%s AND i.path=%s "
            "ORDER BY a.id",
            (project_id, kind, image_path),
        )
        return [{"cls": r[0], "box": [r[1], r[2], r[3], r[4]]} for r in cur.fetchall()]


def write_boxes(input_dir: str, kind: str, image_path: str, boxes: list[dict],
                 created_by: str | None = None, merge: bool = False) -> list[str]:
    """Fully replaces this image's annotation set with `boxes` (or their
    union with what's already saved, if merge=True) -- same replace-vs-merge
    contract /api/label, /api/relabel and /api/testset/label always had.
    `boxes` may be empty (a legitimate "nothing here" for relabel). New
    class names are get-or-created (append-only). Does NOT touch
    `images.status` -- callers that mean "this image just got manually
    labeled" call mark_labeled() themselves (relabel deliberately doesn't,
    same as before). Returns the project's `kind` class list after the
    write."""
    project_id = get_or_create_project(input_dir)
    with db.connect() as conn, conn.cursor() as cur:
        if merge:
            cur.execute(
                "SELECT c.name, a.x1, a.y1, a.x2, a.y2 FROM annotations a "
                "JOIN images i ON i.id = a.image_id "
                "JOIN classes c ON c.id = a.class_id "
                "WHERE i.project_id=%s AND i.kind=%s AND i.path=%s",
                (project_id, kind, image_path),
            )
            boxes = [{"cls": r[0], "box": [r[1], r[2], r[3], r[4]]} for r in cur.fetchall()] + boxes
        image_id = _get_or_create_image(cur, project_id, kind, image_path)
        cur.execute("DELETE FROM annotations WHERE image_id=%s", (image_id,))
        for b in boxes:
            class_id = _get_or_create_class(cur, project_id, kind, b["cls"])
            x1, y1, x2, y2 = b["box"]
            cur.execute(
                "INSERT INTO annotations (image_id, class_id, x1, y1, x2, y2, created_by) "
                "VALUES (%s, %s, %s, %s, %s, %s, %s)",
                (image_id, class_id, x1, y1, x2, y2, created_by),
            )
        cur.execute(
            "SELECT name FROM classes WHERE project_id=%s AND kind=%s ORDER BY idx",
            (project_id, kind),
        )
        return [r[0] for r in cur.fetchall()]


def mark_labeled(input_dir: str, kind: str, image_path: str) -> None:
    project_id = get_or_create_project(input_dir)
    with db.connect() as conn, conn.cursor() as cur:
        image_id = _get_or_create_image(cur, project_id, kind, image_path)
        cur.execute("UPDATE images SET status='labeled' WHERE id=%s", (image_id,))


def mark_auto(input_dir: str, image_paths: list[str]) -> None:
    """Pool only. Never downgrades an image already 'labeled' (by hand) or
    already 'auto' -- the same guard Bank.mark_auto used to apply
    (`if p not in self.auto and p not in self.labeled`)."""
    project_id = get_or_create_project(input_dir)
    with db.connect() as conn, conn.cursor() as cur:
        for p in image_paths:
            image_id = _get_or_create_image(cur, project_id, "pool", p)
            cur.execute(
                "UPDATE images SET status='auto' WHERE id=%s AND status='unlabeled'",
                (image_id,),
            )


def list_by_status(input_dir: str, kind: str) -> dict[str, list[str]]:
    """{"labeled": [...], "auto": [...]} for Bank.summary()."""
    project_id = get_or_create_project(input_dir)
    with db.connect() as conn, conn.cursor() as cur:
        cur.execute(
            "SELECT path, status FROM images WHERE project_id=%s AND kind=%s AND status != 'unlabeled'",
            (project_id, kind),
        )
        rows = cur.fetchall()
    return {
        "labeled": sorted(p for p, s in rows if s == "labeled"),
        "auto": sorted(p for p, s in rows if s == "auto"),
    }


def list_test_images(input_dir: str) -> list[str]:
    project_id = get_or_create_project(input_dir)
    with db.connect() as conn, conn.cursor() as cur:
        cur.execute(
            "SELECT path FROM images WHERE project_id=%s AND kind='testset' ORDER BY path",
            (project_id,),
        )
        return [r[0] for r in cur.fetchall()]


def is_test(input_dir: str, image_path: str) -> bool:
    """Whether `image_path` is held out -- pool endpoints that would teach
    the bank must check this and refuse, or a test image silently stops
    measuring generalization."""
    project_id = get_or_create_project(input_dir)
    with db.connect() as conn, conn.cursor() as cur:
        cur.execute(
            "SELECT 1 FROM images WHERE project_id=%s AND kind='testset' AND path=%s",
            (project_id, image_path),
        )
        return cur.fetchone() is not None


def mark_test(input_dir: str, image_paths: list[str]) -> list[str]:
    """Flags pool images as held-out test set -- no row/file copy of the
    image, just a second `images` row under kind='testset' sharing the same
    path. Skips paths already flagged. Returns the paths actually added."""
    project_id = get_or_create_project(input_dir)
    added = []
    with db.connect() as conn, conn.cursor() as cur:
        for p in image_paths:
            cur.execute(
                "SELECT 1 FROM images WHERE project_id=%s AND kind='testset' AND path=%s",
                (project_id, p),
            )
            if cur.fetchone():
                continue
            cur.execute(
                "INSERT INTO images (project_id, kind, path) VALUES (%s, 'testset', %s)",
                (project_id, p),
            )
            added.append(p)
    return added


def unmark_test(input_dir: str, image_paths: list[str]) -> list[str]:
    """Drops images out of the test set: the image row (and its ground-truth
    annotations, via ON DELETE CASCADE) are gone; the pool row and the image
    file itself are untouched -- there was never a copy to delete."""
    project_id = get_or_create_project(input_dir)
    removed = []
    with db.connect() as conn, conn.cursor() as cur:
        for p in image_paths:
            cur.execute(
                "DELETE FROM images WHERE project_id=%s AND kind='testset' AND path=%s RETURNING id",
                (project_id, p),
            )
            if cur.fetchone():
                removed.append(p)
    return removed


def labeled_stems(input_dir: str) -> set[str]:
    """Stems of testset images that have at least one annotation -- the DB
    equivalent of "a labels/<stem>.txt file exists". Testset-only: pool
    labeled/auto status is tracked via list_by_status() instead."""
    project_id = get_or_create_project(input_dir)
    with db.connect() as conn, conn.cursor() as cur:
        cur.execute(
            "SELECT DISTINCT i.path FROM images i JOIN annotations a ON a.image_id = i.id "
            "WHERE i.project_id=%s AND i.kind='testset'",
            (project_id,),
        )
        return {Path(r[0]).stem for r in cur.fetchall()}


def load_annotations(input_dir: str, kind: str) -> dict[str, list[dict]]:
    """{image_path: [{"cls": name, "box": [x1,y1,x2,y2]}]} for every image in
    `kind` that has at least one box -- one query instead of N read_boxes()
    calls. Used by services/metrics.py::load_ground_truth_db (kind='testset')
    and routers/export.py (either kind)."""
    project_id = get_or_create_project(input_dir)
    with db.connect() as conn, conn.cursor() as cur:
        cur.execute(
            "SELECT i.path, c.name, a.x1, a.y1, a.x2, a.y2 FROM images i "
            "JOIN annotations a ON a.image_id = i.id "
            "JOIN classes c ON c.id = a.class_id "
            "WHERE i.project_id=%s AND i.kind=%s "
            "ORDER BY i.path, a.id",
            (project_id, kind),
        )
        out: dict[str, list[dict]] = {}
        for path, cls, x1, y1, x2, y2 in cur.fetchall():
            out.setdefault(path, []).append({"cls": cls, "box": [x1, y1, x2, y2]})
        return out


def delete_project(input_dir: str) -> None:
    """Dev/test convenience: drops a project and everything under it
    (classes, images, annotations all cascade). Not exposed over HTTP."""
    with db.connect() as conn, conn.cursor() as cur:
        cur.execute("DELETE FROM projects WHERE input_dir=%s", (input_dir,))


def demo():
    """Self-check against a real Postgres (DATABASE_URL) -- covers the
    invariant this migration exists to protect under concurrency: two
    "writers" teaching different new classes to the same project must never
    collide on `idx`."""
    import threading

    db.init_schema()
    project = "/tmp/annotations_db_selfcheck"
    delete_project(project)

    # append-only, race-safe class creation
    ids = []
    get_or_create_class(project, "pool", "warmup")  # pre-create the project row itself
    names = [f"race_{i}" for i in range(8)]
    threads = [threading.Thread(target=lambda n=n: ids.append(get_or_create_class(project, "pool", n)))
               for n in names]
    for t in threads:
        t.start()
    for t in threads:
        t.join()
    assert len(set(ids)) == len(ids), f"class ids collided under concurrent creation: {ids}"
    classes = get_classes(project, "pool")
    assert classes[0] == "warmup" and set(classes[1:]) == set(names), classes

    # pool vs testset are separate index spaces
    write_boxes(project, "pool", "img1.jpg", [{"cls": "a", "box": [0, 0, 10, 10]}])
    write_boxes(project, "testset", "img1.jpg", [{"cls": "a", "box": [1, 1, 9, 9]}])
    assert get_classes(project, "pool")[-1] == "a"
    assert get_classes(project, "testset") == ["a"]
    assert read_boxes(project, "pool", "img1.jpg") == [{"cls": "a", "box": [0.0, 0.0, 10.0, 10.0]}]
    assert read_boxes(project, "testset", "img1.jpg") == [{"cls": "a", "box": [1.0, 1.0, 9.0, 9.0]}]

    # replace vs merge
    write_boxes(project, "pool", "img1.jpg", [{"cls": "a", "box": [5, 5, 6, 6]}])
    assert len(read_boxes(project, "pool", "img1.jpg")) == 1
    write_boxes(project, "pool", "img1.jpg", [{"cls": "a", "box": [7, 7, 8, 8]}], merge=True)
    assert len(read_boxes(project, "pool", "img1.jpg")) == 2

    # status tracking
    mark_labeled(project, "pool", "img1.jpg")
    mark_auto(project, ["img2.jpg"])
    mark_auto(project, ["img1.jpg"])  # already labeled -> must not downgrade to 'auto'
    status = list_by_status(project, "pool")
    assert status == {"labeled": ["img1.jpg"], "auto": ["img2.jpg"]}, status

    # testset flagging + isolation
    assert mark_test(project, ["img3.jpg"]) == ["img3.jpg"]
    assert mark_test(project, ["img3.jpg"]) == []  # already flagged -> no-op
    assert is_test(project, "img3.jpg") and not is_test(project, "img2.jpg")
    write_boxes(project, "testset", "img3.jpg", [{"cls": "a", "box": [0, 0, 1, 1]}])
    assert labeled_stems(project) == {"img1", "img3"}
    assert load_annotations(project, "testset") == {
        "img1.jpg": [{"cls": "a", "box": [1.0, 1.0, 9.0, 9.0]}],
        "img3.jpg": [{"cls": "a", "box": [0.0, 0.0, 1.0, 1.0]}],
    }
    assert unmark_test(project, ["img3.jpg"]) == ["img3.jpg"]
    assert not is_test(project, "img3.jpg")
    assert labeled_stems(project) == {"img1"}  # ground truth gone with the flag

    delete_project(project)
    print("annotations_db self-check OK")


if __name__ == "__main__":
    demo()
