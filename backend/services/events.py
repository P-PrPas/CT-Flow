"""Session events, so §7's effort metrics survive a browser reload.

The UI already counts "how long did that image take" and "how many auto
labels did I have to fix" while you work -- but only in React state, which
means the answer to "is this tool actually saving us time" dies with the tab.
This appends each event next to the bank it belongs to:

    <output_dir>/_bank/events.jsonl

One JSON object per line, append-only. ponytail: a text file and a `for` loop,
no database and no analytics service -- these are a handful of counters per
labeling day, and the file lands in the same folder as the dataset it
describes, which is where anyone asking the question will look.
"""
import json
import time
from pathlib import Path
from statistics import median

# Kinds the summary understands. Anything else is still recorded (the file is
# the raw log) but does not move a number.
SESSION, LABEL, FIX, AUTO = "session", "label", "fix", "auto"


def path(output_dir: str) -> Path:
    return Path(output_dir) / "_bank" / "events.jsonl"


def append(output_dir: str, event: dict) -> None:
    # ponytail: one short line, opened in append mode -- concurrent writers
    # interleave whole lines rather than corrupting each other on every OS we
    # run on. Reach for Bank.lock if an event ever grows past a few hundred bytes.
    p = path(output_dir)
    p.parent.mkdir(parents=True, exist_ok=True)
    # Server clock, not the browser's -- the same reason jobs.py reports `now`.
    stamped = {"ts": time.time()} | event
    with p.open("a", encoding="utf-8") as f:
        f.write(json.dumps(stamped, ensure_ascii=False) + "\n")


def read(output_dir: str) -> list[dict]:
    p = path(output_dir)
    if not p.exists():
        return []
    out = []
    for line in p.read_text(encoding="utf-8").splitlines():
        try:
            out.append(json.loads(line))
        except json.JSONDecodeError:
            continue  # a half-written last line loses one event, not the log
    return out


def _num(values: list[float]) -> float | None:
    return round(median(values), 1) if values else None


def summary(output_dir: str) -> dict:
    """The §7 table, computed from the log. Every field is None when nothing
    has been recorded yet -- an honest "not measured" rather than a zero that
    reads like a real measurement."""
    ev = read(output_dir)
    kinds = lambda k: [e for e in ev if e.get("kind") == k]  # noqa: E731

    sessions = kinds(SESSION)
    autos = kinds(AUTO)
    labels = kinds(LABEL)
    fixes = kinds(FIX)

    # Session abandonment: opened a session, never got as far as auto-label.
    started = {e.get("session") for e in sessions if e.get("session")}
    reached_auto = {e.get("session") for e in autos if e.get("session")}
    written = sum(e.get("written", 0) for e in autos)

    return {
        "sessions": len(started),
        "sessions_reaching_autolabel": len(reached_auto & started),
        "abandonment": (round(1 - len(reached_auto & started) / len(started), 3)
                        if started else None),
        "manual_labels": len(labels),
        "median_label_secs": _num([e["secs"] for e in labels if isinstance(e.get("secs"), (int, float))]),
        "median_time_to_first_auto_secs": _num(
            [e["secs"] for e in autos if isinstance(e.get("secs"), (int, float))]
        ),
        "auto_written": written,
        "corrections": len(fixes),
        "correction_rate": round(len(fixes) / written, 3) if written else None,
    }


def demo():
    import shutil
    import tempfile

    d = tempfile.mkdtemp()
    try:
        assert summary(d) == {
            "sessions": 0, "sessions_reaching_autolabel": 0, "abandonment": None,
            "manual_labels": 0, "median_label_secs": None,
            "median_time_to_first_auto_secs": None, "auto_written": 0,
            "corrections": 0, "correction_rate": None,
        }, summary(d)

        for e in [
            {"kind": SESSION, "session": "s1"},
            {"kind": SESSION, "session": "s2"},          # abandoned: never auto-labels
            {"kind": LABEL, "session": "s1", "secs": 10},
            {"kind": LABEL, "session": "s1", "secs": 20},
            {"kind": LABEL, "session": "s1", "secs": 30},
            {"kind": AUTO, "session": "s1", "secs": 900, "written": 10},
            {"kind": FIX, "session": "s1"},
            {"kind": FIX, "session": "s1"},
        ]:
            append(d, e)

        s = summary(d)
        assert s["sessions"] == 2 and s["sessions_reaching_autolabel"] == 1
        assert s["abandonment"] == 0.5, s
        assert s["median_label_secs"] == 20 and s["manual_labels"] == 3, s
        assert s["median_time_to_first_auto_secs"] == 900, s
        assert s["correction_rate"] == 0.2, s          # 2 fixes / 10 auto labels

        # A truncated last line must cost one event, not the whole file.
        with path(d).open("a", encoding="utf-8") as f:
            f.write('{"kind": "label", "secs":')
        assert summary(d)["manual_labels"] == 3, summary(d)

        # An auto-label run from a session we never saw open must not push
        # abandonment below zero.
        append(d, {"kind": AUTO, "session": "unknown", "written": 5})
        assert summary(d)["abandonment"] == 0.5, summary(d)
        print("events self-check OK")
    finally:
        shutil.rmtree(d)


if __name__ == "__main__":
    demo()
