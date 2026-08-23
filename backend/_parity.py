"""Diff two running backends endpoint by endpoint.

The smoke test answers "does this backend work". This answers the question that
actually matters during the strangler phase (docs/REFACTOR_PLAN.md phase 2):
"does the Go one answer *identically* to the Python one". A port can satisfy
every assertion in the smoke test and still round a float differently, order a
list differently, or drop a field the UI reads.

    python -m backend._parity --a http://localhost:8100 --b http://localhost:8000 \
        --pool-a /data/_parity_a --pool-b /data/_parity_b

Each side gets its own copy of the fixture pool, because they share one
PostgreSQL and one input_dir is one project row -- running both through the
same directory would have them overwriting each other's answers. Pool paths are
normalised out of the responses before comparing, so "same inputs modulo the
directory" is what gets checked.

Exit status is 0 only when every step matches. Volatile fields (job ids, server
clocks) are dropped rather than compared; everything else, including every
float, has to agree.
"""
import argparse
import hashlib
import io
import json
import shutil
import sys
import time
import zipfile
from pathlib import Path

import httpx

HERE = Path(__file__).resolve().parent
FIXTURE = HERE / "test_pool"

# Wall-clock and identity fields differ between two processes by definition.
# Anything not listed here is compared -- a new field in a response is a
# difference worth failing on, not something to quietly ignore.
VOLATILE = {"job_id", "started_at", "now"}
TOLERANCE = 1e-9


def _norm(value, pool: str):
    """Replace this side's pool path with a placeholder, everywhere it appears
    -- as a value, and as a dict key (score/eval results are keyed by image
    path)."""
    if isinstance(value, str):
        return value.replace(pool, "<POOL>")
    if isinstance(value, list):
        return [_norm(v, pool) for v in value]
    if isinstance(value, dict):
        return {_norm(k, pool): _norm(v, pool) for k, v in value.items() if k not in VOLATILE}
    return value


def _diff(a, b, path="") -> list[str]:
    if isinstance(a, float) or isinstance(b, float):
        if isinstance(a, (int, float)) and isinstance(b, (int, float)):
            return [] if abs(a - b) <= TOLERANCE else [f"{path}: {a} != {b}"]
    if type(a) is not type(b):
        return [f"{path}: type {type(a).__name__} != {type(b).__name__} ({a!r} / {b!r})"]
    if isinstance(a, dict):
        out = []
        for k in sorted(set(a) | set(b)):
            if k not in a:
                out.append(f"{path}.{k}: missing on A (B has {b[k]!r})")
            elif k not in b:
                out.append(f"{path}.{k}: missing on B (A has {a[k]!r})")
            else:
                out += _diff(a[k], b[k], f"{path}.{k}")
        return out
    if isinstance(a, list):
        if len(a) != len(b):
            return [f"{path}: length {len(a)} != {len(b)}"]
        return [d for i, (x, y) in enumerate(zip(a, b)) for d in _diff(x, y, f"{path}[{i}]")]
    return [] if a == b else [f"{path}: {a!r} != {b!r}"]


class Side:
    def __init__(self, url: str, pool: str):
        self.c = httpx.Client(base_url=url, timeout=180)
        self.pool = pool
        self.url = url

    def call(self, method: str, path: str, **kw):
        r = self.c.request(method, path, **kw)
        try:
            body = r.json()
        except ValueError:
            body = self._binary(r)
        return {"status": r.status_code, "body": _norm(body, self.pool)}

    @staticmethod
    def _binary(r) -> dict:
        """Not every endpoint answers JSON. An image is compared by hash, but a
        zip cannot be: the container records a modification time per member, so
        two archives with identical contents never hash the same. Compare what
        the archive *says* instead -- which is also the only part a consumer of
        the export reads."""
        ctype = r.headers.get("content-type")
        if ctype == "application/zip":
            with zipfile.ZipFile(io.BytesIO(r.content)) as zf:
                return {"_content_type": ctype,
                        "_zip": {n: zf.read(n).decode("utf-8", "replace")
                                 for n in sorted(zf.namelist())}}
        return {"_bytes_sha": hashlib.sha256(r.content).hexdigest(), "_content_type": ctype}

    def job(self, path: str, payload: dict):
        """Start a background job and return its finished result -- the job id
        and progress ticks are per-process, only the outcome is comparable."""
        r = self.c.post(path, json=payload)
        if r.status_code != 200:
            return {"status": r.status_code, "body": _norm(r.json(), self.pool)}
        started = r.json()
        deadline = time.time() + 180
        while True:
            j = self.c.get(f"/api/jobs/{started['job_id']}").json()
            if j["finished"]:
                return {"status": 200, "total": started["total"],
                        "body": _norm({"result": j["result"], "error": j["error"]}, self.pool)}
            assert time.time() < deadline, f"{self.url} job never finished"
            time.sleep(0.2)


def steps(pool: str):
    """One scripted pass over every endpoint, in an order that leaves the
    project in a state the next step can use. `pool` is substituted per side."""
    img = lambda n: f"{pool}/{sorted(p.name for p in FIXTURE.glob('*.jpg'))[n]}"
    box = lambda cls, b: {"cls": cls, "box": b}
    return [
        ("config", "GET", "/api/config", {}),
        ("browse", "GET", "/api/browse", {"params": {"path": pool}}),
        ("browse-missing", "GET", "/api/browse", {"params": {"path": f"{pool}/nope"}}),
        ("browse-roots", "GET", "/api/browse", {"params": {"path": ""}}),
        ("auth-me", "GET", "/api/auth/me", {}),
        ("session", "POST", "/api/session", {"json": {"input_dir": pool}}),
        ("session-missing", "POST", "/api/session", {"json": {"input_dir": f"{pool}/nope"}}),
        ("image", "GET", "/api/image", {"params": {"path": img(0)}}),
        ("image-404", "GET", "/api/image", {"params": {"path": f"{pool}/nope.jpg"}}),
        ("label", "POST", "/api/label",
         {"json": {"input_dir": pool, "image": img(0), "boxes": [box("widget", [30, 30, 120, 120])]}}),
        ("label-model-mismatch", "POST", "/api/label",
         {"json": {"input_dir": pool, "image": img(0), "boxes": [box("widget", [1, 1, 5, 5])],
                   "model_id": "yoloe-11m-seg"}}),
        ("label-no-boxes", "POST", "/api/label",
         {"json": {"input_dir": pool, "image": img(0), "boxes": []}}),
        ("boxes", "GET", "/api/boxes", {"params": {"input_dir": pool, "image": img(0)}}),
        ("label-second-class", "POST", "/api/label",
         {"json": {"input_dir": pool, "image": img(1), "boxes": [box("aaa_other", [1, 1, 40, 40])]}}),
        ("label-update", "POST", "/api/label",
         {"json": {"input_dir": pool, "image": img(0), "boxes": [box("widget", [200, 200, 260, 260])],
                   "mode": "update"}}),
        ("boxes-after-update", "GET", "/api/boxes", {"params": {"input_dir": pool, "image": img(0)}}),
        ("relabel-unknown-class", "POST", "/api/relabel",
         {"json": {"input_dir": pool, "image": img(0), "boxes": [box("never-taught", [1, 1, 2, 2])]}}),
        ("relabel-empty", "POST", "/api/relabel",
         {"json": {"input_dir": pool, "image": img(0), "boxes": []}}),
        ("predict", "POST", "/api/predict",
         {"json": {"input_dir": pool, "image": img(0), "conf": 0.05}}),
        ("predict-conf-by-class", "POST", "/api/predict",
         {"json": {"input_dir": pool, "image": img(0), "conf": 0.05,
                   "conf_by_class": {"widget": 0.9}}}),
        ("testset-import", "POST", "/api/testset/import",
         {"json": {"input_dir": pool, "images": [img(2)]}}),
        ("testset-import-again", "POST", "/api/testset/import",
         {"json": {"input_dir": pool, "images": [img(2)]}}),
        ("testset-label", "POST", "/api/testset/label",
         {"json": {"input_dir": pool, "image": img(2), "boxes": [box("widget", [40, 40, 150, 150])]}}),
        ("testset-label-not-imported", "POST", "/api/testset/label",
         {"json": {"input_dir": pool, "image": img(0), "boxes": [box("widget", [1, 1, 2, 2])]}}),
        ("label-into-testset", "POST", "/api/label",
         {"json": {"input_dir": pool, "image": img(2), "boxes": [box("widget", [1, 1, 9, 9])]}}),
        ("boxes-testset", "GET", "/api/boxes",
         {"params": {"input_dir": pool, "image": img(2), "kind": "test"}}),
        ("history-empty", "GET", "/api/history", {"params": {"input_dir": pool}}),
        ("history-add", "POST", "/api/history",
         {"json": {"input_dir": pool, "point": {"ts": 1, "conf": 0.25, "f1": 0.5}}}),
        ("history-read", "GET", "/api/history", {"params": {"input_dir": pool}}),
        ("history-clear", "DELETE", "/api/history", {"params": {"input_dir": pool}}),
        ("events-add", "POST", "/api/events",
         {"json": {"input_dir": pool, "kind": "label", "session": "p1", "secs": 12, "written": 1}}),
        ("events-summary", "GET", "/api/events", {"params": {"input_dir": pool}}),
        ("export-yolo", "GET", "/api/export", {"params": {"input_dir": pool, "format": "yolo"}}),
        ("export-coco", "GET", "/api/export", {"params": {"input_dir": pool, "format": "coco"}}),
        ("export-voc", "GET", "/api/export", {"params": {"input_dir": pool, "format": "voc"}}),
        ("export-bad-format", "GET", "/api/export", {"params": {"input_dir": pool, "format": "xml"}}),
        ("export-testset", "GET", "/api/export",
         {"params": {"input_dir": pool, "format": "coco", "kind": "testset"}}),
        ("jobs-unknown", "GET", "/api/jobs/does-not-exist", {}),
        ("path-escape", "POST", "/api/session", {"json": {"input_dir": "/etc"}}),
    ]


JOBS = [
    ("job-score", "/api/score", lambda pool, img: {"input_dir": pool, "images": [img(0), img(1)]}),
    ("job-evaluate", "/api/evaluate", lambda pool, img: {"input_dir": pool, "conf": 0.05}),
    ("job-autolabel", "/api/autolabel",
     lambda pool, img: {"input_dir": pool, "images": [img(1)], "conf": 0.05}),
    ("job-reembed", "/api/reembed", lambda pool, img: {"input_dir": pool, "model_id": "yoloe-26s-seg"}),
]


def prepare(pool: str):
    """A fresh copy of the fixture pool, with no leftover .ctflow state."""
    p = Path(pool)
    if p.exists():
        shutil.rmtree(p)
    p.mkdir(parents=True)
    for f in FIXTURE.glob("*.jpg"):
        shutil.copy(f, p / f.name)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--a", required=True, help="baseline base URL (the Python service)")
    ap.add_argument("--b", required=True, help="candidate base URL (the Go service)")
    ap.add_argument("--pool-a", required=True)
    ap.add_argument("--pool-b", required=True)
    ap.add_argument("--only", help="substring filter over step names")
    ap.add_argument("--skip-jobs", action="store_true",
                    help="skip the inference passes -- much faster while porting a read-only group")
    args = ap.parse_args()

    prepare(args.pool_a)
    prepare(args.pool_b)
    a, b = Side(args.a, args.pool_a), Side(args.b, args.pool_b)

    plan = [(n, lambda s, m=m, p=p, k=k: s.call(m, p, **_sub(k, s.pool)))
            for n, m, p, k in steps("<POOL>")]
    if not args.skip_jobs:
        img = lambda s: (lambda i: f"{s.pool}/{sorted(p.name for p in FIXTURE.glob('*.jpg'))[i]}")
        plan += [(n, lambda s, p=path, f=f: s.job(p, f(s.pool, img(s)))) for n, path, f in JOBS]

    failures, ran = [], 0
    for name, run in plan:
        if args.only and args.only not in name:
            continue
        ran += 1
        ra, rb = run(a), run(b)
        d = _diff(ra, rb, name)
        if d:
            failures.append((name, d))
            print(f"DIFF {name}")
            for line in d[:12]:
                print(f"       {line}")
            if len(d) > 12:
                print(f"       ... and {len(d) - 12} more")
        else:
            print(f"  ok {name}")

    print(f"\n{ran - len(failures)}/{ran} identical")
    if failures:
        print("PARITY FAILED: " + ", ".join(n for n, _ in failures))
        return 1
    print("PARITY OK")
    return 0


def _sub(kw: dict, pool: str) -> dict:
    """steps() is built once with a <POOL> placeholder; each side substitutes
    its own directory in, so both get byte-identical requests otherwise."""
    return json.loads(json.dumps(kw).replace("<POOL>", pool))


if __name__ == "__main__":
    sys.exit(main())
