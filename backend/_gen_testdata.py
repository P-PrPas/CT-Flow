"""Regenerate the cross-language golden vectors in backend/testdata/.

These files are the contract the Go port is held to (docs/REFACTOR_PLAN.md
phase 0): Python produces them, Go's unit tests must reproduce them exactly.
That is the only practical way to check a pure function -- a pbkdf2 hash, an
F1 score, a COCO document -- across two languages without standing both
implementations up and diffing over HTTP.

    python -m backend._gen_testdata          # rewrites backend/testdata/

Regenerate ONLY when the behaviour is meant to change. A diff here is either a
deliberate spec change or the bug this directory exists to catch, and the
commit message has to say which.

Zip archives are stored decoded (member name -> text) rather than as bytes:
zip embeds mtimes, so the container is not reproducible even when its contents
are, and it is the contents both languages have to agree on.
"""
import io
import json
import zipfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
OUT = HERE / "testdata"
POOL = HERE / "test_pool"


def gen_auth() -> dict:
    """pbkdf2 hashes and signed session cookies. Both formats are frozen: an
    existing LABEL_TOOL_USERS entry has to keep working after the swap, and a
    cookie issued by one implementation has to be accepted by the other --
    during the strangler phase a login and the request after it can land on
    different processes."""
    import os

    secret = "0" * 64  # fixed so the signatures below are reproducible
    os.environ["LABEL_TOOL_SECRET"] = secret
    from .services import auth

    auth._SECRET = secret.encode()  # module already read the env at import time

    stored = auth.hash_password("hunter2")
    other = auth.hash_password("correct horse battery staple")
    return {
        "iterations": auth.ITERATIONS,
        "cookie_name": auth.COOKIE,
        "ttl_seconds": auth.TTL_SECONDS,
        "verify": [
            {"password": "hunter2", "stored": stored, "expect": True},
            {"password": "hunter3", "stored": stored, "expect": False},
            {"password": "", "stored": stored, "expect": False},
            {"password": "correct horse battery staple", "stored": other, "expect": True},
            {"password": "hunter2", "stored": "garbage", "expect": False},
            {"password": "hunter2", "stored": "bcrypt$1$aa$bb", "expect": False},
        ],
        "secret": secret,
        "identify": [
            # far-future expiry, correctly signed
            {"token": f"alice|9999999999|{auth._sign('alice|9999999999')}",
             "expect": "alice", "why": "valid"},
            # a username containing the separator must round-trip: identify()
            # splits from the right so it can never shift the expiry field
            {"token": f"a|b|9999999999|{auth._sign('a|b|9999999999')}",
             "expect": "a|b", "why": "username contains the separator"},
            {"token": f"alice|1|{auth._sign('alice|1')}",
             "expect": None, "why": "correctly signed but expired"},
            {"token": "alice|9999999999|deadbeef", "expect": None, "why": "forged signature"},
            {"token": "nonsense", "expect": None, "why": "malformed"},
            {"token": "", "expect": None, "why": "absent"},
            {"token": f"alice|notanumber|{auth._sign('alice|notanumber')}",
             "expect": None, "why": "signed, but expiry is not an integer"},
        ],
    }


def gen_metrics() -> dict:
    """Greedy IoU>=0.5 matching, per-class and overall precision/recall/F1.
    Covers the cases that separate a correct matcher from one that merely
    passes the happy path: a second prediction on an already-matched box, a
    right-place-wrong-class detection, and IoU landing either side of the
    threshold."""
    from .services import metrics

    cases = [
        {"name": "perfect match",
         "gt": {"a.jpg": [{"cls": "x", "box": [10, 10, 50, 50]}]},
         "pred": {"a.jpg": [{"cls": "x", "box": [10, 10, 50, 50], "conf": 0.9}]}},
        {"name": "missed detection (fn) and a spurious one (fp)",
         "gt": {"a.jpg": [{"cls": "x", "box": [10, 10, 50, 50]}]},
         "pred": {"a.jpg": [{"cls": "x", "box": [200, 200, 240, 240], "conf": 0.7}]}},
        {"name": "right place, wrong class -- never a match",
         "gt": {"a.jpg": [{"cls": "x", "box": [10, 10, 50, 50]}]},
         "pred": {"a.jpg": [{"cls": "y", "box": [10, 10, 50, 50], "conf": 0.8}]}},
        {"name": "two predictions on one gt -- the lower-scoring one is a fp",
         "gt": {"a.jpg": [{"cls": "x", "box": [10, 10, 50, 50]}]},
         "pred": {"a.jpg": [{"cls": "x", "box": [11, 11, 51, 51], "conf": 0.6},
                            {"cls": "x", "box": [10, 10, 50, 50], "conf": 0.95}]}},
        {"name": "IoU either side of 0.5",
         "gt": {"a.jpg": [{"cls": "x", "box": [0, 0, 100, 100]}],
                "b.jpg": [{"cls": "x", "box": [0, 0, 100, 100]}]},
         "pred": {"a.jpg": [{"cls": "x", "box": [0, 0, 100, 71], "conf": 0.9}],
                  "b.jpg": [{"cls": "x", "box": [0, 0, 100, 69], "conf": 0.9}]}},
        {"name": "two classes, mixed outcomes",
         "gt": {"a.jpg": [{"cls": "x", "box": [10, 10, 50, 50]},
                          {"cls": "y", "box": [60, 60, 90, 90]}],
                "b.jpg": [{"cls": "y", "box": [0, 0, 20, 20]}]},
         "pred": {"a.jpg": [{"cls": "x", "box": [10, 10, 50, 50], "conf": 0.9},
                            {"cls": "y", "box": [61, 61, 91, 91], "conf": 0.5}],
                  "b.jpg": []}},
        {"name": "empty everywhere -- zeroes, not a divide by zero",
         "gt": {"a.jpg": []}, "pred": {"a.jpg": []}},
    ]
    for case in cases:
        case["want"] = metrics.evaluate(case["gt"], case["pred"])

    return {
        "iou": [
            {"a": [0, 0, 10, 10], "b": [0, 0, 10, 10], "want": metrics.iou([0, 0, 10, 10], [0, 0, 10, 10])},
            {"a": [0, 0, 10, 10], "b": [20, 20, 30, 30], "want": metrics.iou([0, 0, 10, 10], [20, 20, 30, 30])},
            {"a": [0, 0, 10, 10], "b": [5, 5, 15, 15], "want": metrics.iou([0, 0, 10, 10], [5, 5, 15, 15])},
            {"a": [0, 0, 10, 10], "b": [10, 10, 20, 20], "want": metrics.iou([0, 0, 10, 10], [10, 10, 20, 20])},
            {"a": [0, 0, 0, 0], "b": [0, 0, 0, 0], "want": metrics.iou([0, 0, 0, 0], [0, 0, 0, 0])},
        ],
        "cases": cases,
    }


def gen_events(tmp: Path) -> dict:
    """The §7 effort summary. Its one real rule is that "not measured" is null
    and "measured zero" is 0.0 -- a port that collapses both to zero passes a
    casual read and destroys the only number the metric exists to report."""
    from .services import events

    log = [
        {"kind": "session", "session": "s1", "secs": None, "written": 0},
        {"kind": "label", "session": "s1", "secs": 12.0, "written": 1},
        {"kind": "label", "session": "s1", "secs": 8.0, "written": 1},
        {"kind": "label", "session": "s1", "secs": 30.0, "written": 1},
        {"kind": "auto", "session": "s1", "secs": 300.0, "written": 4},
        {"kind": "fix", "session": "s1", "secs": None, "written": 1},
        {"kind": "session", "session": "s2", "secs": None, "written": 0},
        {"kind": "label", "session": "s2", "secs": 20.0, "written": 1},
        {"kind": "not-a-kind", "session": "s2", "secs": 1.0, "written": 99},
    ]
    d = tmp / "events_fixture"
    (d / "_bank").mkdir(parents=True, exist_ok=True)
    (d / "_bank" / "events.jsonl").write_text(
        "".join(json.dumps(e) + "\n" for e in log), encoding="utf-8"
    )
    both = {"log": log, "want": events.summary(str(d))}

    empty = tmp / "events_empty"
    empty.mkdir(parents=True, exist_ok=True)
    both["want_empty"] = events.summary(str(empty))
    return both


def _unzip(raw: bytes) -> dict:
    with zipfile.ZipFile(io.BytesIO(raw)) as zf:
        return {n: zf.read(n).decode("utf-8") for n in sorted(zf.namelist())}


def gen_export() -> dict:
    """YOLO/COCO/VOC serialisation. Pixel coords come out of the database; only
    yolo and voc reopen the image, for its dimensions -- so these vectors also
    pin the normalisation arithmetic and the float formatting that goes with
    it, which is exactly where two languages drift apart."""
    from .routers import export

    files = sorted(p.name for p in POOL.glob("*.jpg"))
    names = ["test_item", "aaa_new_class"]
    # Keyed by basename, not absolute path: the checkout lives somewhere else on
    # every machine, and every field the exporters emit is derived from the
    # basename or stem anyway. Both sides join these against their own
    # backend/test_pool before running.
    by_name = {
        files[0]: [{"cls": "test_item", "box": [30.0, 30.0, 120.0, 120.0]},
                   {"cls": "aaa_new_class", "box": [1.5, 2.5, 20.25, 40.75]}],
        files[1]: [{"cls": "aaa_new_class", "box": [0.0, 0.0, 10.0, 10.0]}],
        # a path that no longer exists is skipped, not fatal to the whole export
        "deleted_since_it_was_labelled.jpg": [
            {"cls": "test_item", "box": [1.0, 1.0, 2.0, 2.0]}],
    }
    by_image = {str(POOL / n): boxes for n, boxes in by_name.items()}
    return {
        "pool_dir": "backend/test_pool",
        "names": names,
        "by_image": by_name,
        "yolo": _unzip(export._export_yolo(names, by_image)),
        "coco": json.loads(export._export_coco(names, by_image).decode("utf-8")),
        "voc": _unzip(export._export_voc(names, by_image)),
    }


def check():
    """Verify the committed vectors against the current Python code -- the same
    thing Go's unit tests do against the same files, which is what keeps the two
    implementations pinned to each other instead of drifting in parallel.

    Semantic, not a re-generate-and-diff: hash_password() salts randomly, so a
    fresh run never reproduces byte-identical vectors. Checking behaviour is
    also what a port actually has to satisfy.
    """
    import os

    data = {p.stem: json.loads(p.read_text(encoding="utf-8")) for p in OUT.glob("*.json")}

    av = data["auth_vectors"]
    os.environ["LABEL_TOOL_SECRET"] = av["secret"]
    from .services import auth

    auth._SECRET = av["secret"].encode()
    assert auth.ITERATIONS == av["iterations"], auth.ITERATIONS
    assert auth.COOKIE == av["cookie_name"] and auth.TTL_SECONDS == av["ttl_seconds"]
    for v in av["verify"]:
        got = auth.verify_password(v["password"], v["stored"])
        assert got == v["expect"], f"verify_password({v['password']!r}) -> {got}"
    for v in av["identify"]:
        got = auth.identify(v["token"] or None)
        assert got == v["expect"], f"identify({v['why']}) -> {got!r}, want {v['expect']!r}"

    from .services import metrics

    for v in data["metrics_cases"]["iou"]:
        got = metrics.iou(v["a"], v["b"])
        assert abs(got - v["want"]) < 1e-12, (v, got)
    for case in data["metrics_cases"]["cases"]:
        got = metrics.evaluate(case["gt"], case["pred"])
        assert got == case["want"], case["name"]

    import tempfile

    from .services import events

    ev = data["events_cases"]
    with tempfile.TemporaryDirectory() as tmp:
        d = Path(tmp) / "e"
        (d / "_bank").mkdir(parents=True)
        (d / "_bank" / "events.jsonl").write_text(
            "".join(json.dumps(e) + "\n" for e in ev["log"]), encoding="utf-8")
        assert events.summary(str(d)) == ev["want"], events.summary(str(d))
        empty = Path(tmp) / "empty"
        empty.mkdir()
        assert events.summary(str(empty)) == ev["want_empty"]

    from .routers import export

    ex = data["export_cases"]
    by_image = {str(POOL / n): b for n, b in ex["by_image"].items()}
    assert _unzip(export._export_yolo(ex["names"], by_image)) == ex["yolo"]
    assert json.loads(export._export_coco(ex["names"], by_image).decode()) == ex["coco"]
    assert _unzip(export._export_voc(ex["names"], by_image)) == ex["voc"]

    print(f"testdata self-check OK ({len(data)} vector files)")


def main():
    import tempfile

    OUT.mkdir(exist_ok=True)
    with tempfile.TemporaryDirectory() as tmp:
        written = {
            "auth_vectors.json": gen_auth(),
            "metrics_cases.json": gen_metrics(),
            "events_cases.json": gen_events(Path(tmp)),
            "export_cases.json": gen_export(),
        }
    for name, payload in written.items():
        (OUT / name).write_text(
            json.dumps(payload, indent=2, ensure_ascii=False, sort_keys=False) + "\n",
            encoding="utf-8",
        )
        print(f"wrote testdata/{name}")


if __name__ == "__main__":
    import sys

    check() if "--check" in sys.argv else main()
