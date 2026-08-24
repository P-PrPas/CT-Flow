"""The cross-language golden vectors in backend/testdata/.

These files were how the Go port was held to Python's behaviour
(docs/REFACTOR_PLAN.md phase 0): Python produced them, and Go's unit tests
reproduce them exactly. That is the only practical way to check a pure function
-- a pbkdf2 hash, an F1 score, a COCO document -- across two languages without
standing both implementations up and diffing over HTTP.

    python -m backend.tests.gen_testdata            # regenerate what still can be
    python -m backend.tests.gen_testdata --check    # verify against current Python

Now that the port is finished, most of these are **frozen**: the Python that
produced them (the FastAPI auth, events and export modules) no
longer exists, so the committed files are the last output it gave -- which is
precisely what makes them the spec. Go's tests are what check them:
internal/platform/auth, internal/infra/events and internal/core/export respectively.

metrics_cases.json is the exception and the important one. tools/metrics.py
is still here and still called by tools/experiment_conf.py, so two implementations of
the readiness score genuinely coexist -- this is what keeps them agreeing, and
it is the only vector file this script can still generate or verify.

Regenerate ONLY when the behaviour is meant to change. A diff is either a
deliberate spec change or the bug this directory exists to catch, and the commit
message has to say which.

Zip archives inside export_cases.json are stored decoded (member name -> text)
rather than as bytes: zip embeds mtimes, so the container is never reproducible
even when its contents are, and it is the contents both languages agree on.
"""
import json
from pathlib import Path

HERE = Path(__file__).resolve().parent
OUT = HERE / "testdata"
POOL = HERE / "fixtures" / "pool"


def gen_metrics() -> dict:
    """Greedy IoU>=0.5 matching, per-class and overall precision/recall/F1.
    Covers the cases that separate a correct matcher from one that merely
    passes the happy path: a second prediction on an already-matched box, a
    right-place-wrong-class detection, and IoU landing either side of the
    threshold."""
    from ..tools import metrics

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


def check():
    """Verify metrics_cases.json against the current tools/metrics.py.

    Semantic, not a re-generate-and-diff. This is what stops the two surviving
    implementations of the readiness score -- this one and internal/core/metrics --
    from drifting apart while both are still in use.

    The other three vector files are not checked here; the Python that produced
    them is gone (see the module docstring), and Go's own tests check those.
    """
    data = {p.stem: json.loads(p.read_text(encoding="utf-8")) for p in OUT.glob("*.json")}

    from ..tools import metrics

    for v in data["metrics_cases"]["iou"]:
        got = metrics.iou(v["a"], v["b"])
        assert abs(got - v["want"]) < 1e-12, (v, got)
    for case in data["metrics_cases"]["cases"]:
        got = metrics.evaluate(case["gt"], case["pred"])
        assert got == case["want"], case["name"]

    frozen = sorted(set(data) - {"metrics_cases"})
    print(f"metrics vectors verified against tools/metrics.py; "
          f"frozen (checked by Go): {', '.join(frozen)}")


def main():
    OUT.mkdir(exist_ok=True)
    written = {"metrics_cases.json": gen_metrics()}
    for name, payload in written.items():
        (OUT / name).write_text(
            json.dumps(payload, indent=2, ensure_ascii=False, sort_keys=False) + "\n",
            encoding="utf-8",
        )
        print(f"wrote testdata/{name}")


if __name__ == "__main__":
    import sys

    check() if "--check" in sys.argv else main()
