"""T-01 -- does the `defect` class have signal hiding under the confidence
threshold, or is it genuinely not being detected?

/api/score runs at conf=0.05 while /api/evaluate defaults to 0.25, a factor of
five. Before anyone invests in T-08 (crop-before-SAVPE) we need to know whether
recall moves when the threshold drops. This builds a prompt bank the same way
the UI does -- ground-truth boxes fed through extract_embedding, grouped by
class per image -- then evaluates the same bank at four thresholds.

    .venv\\Scripts\\python.exe -m backend.tools.experiment_conf [prompts_per_class] [conf ...]

Reads data/conveyor_pvc/{pool,pool_ground_truth,test}; writes nothing outside
a scratch bank it deletes on the way out.
"""
import shutil
import statistics
import sys
from pathlib import Path

import cv2

# Was backend/config.py, which the Go port removed. This script is the only
# thing that needed it, so the one constant lives here now.
REPO_ROOT = Path(__file__).resolve().parents[3]

from . import metrics
from ..inference.bank import Bank
from ..inference.vpe import arm, extract_embedding, predict_one

DATA = REPO_ROOT / "data" / "conveyor_pvc"
POOL, POOL_GT, TEST = DATA / "pool", DATA / "pool_ground_truth", DATA / "test"
SCRATCH = Path(__file__).parent / "_experiment_out"
DEFAULT_CONFS = [0.05, 0.10, 0.15, 0.25]


def read_yolo(txt: Path, w: int, h: int, names: list[str]) -> list[dict]:
    boxes = []
    for line in txt.read_text(encoding="utf-8").splitlines():
        parts = line.split()
        if len(parts) != 5:
            continue
        k, cx, cy, bw, bh = int(parts[0]), *(float(v) for v in parts[1:])
        boxes.append({"cls": names[k],
                      "box": [(cx - bw / 2) * w, (cy - bh / 2) * h,
                              (cx + bw / 2) * w, (cy + bh / 2) * h]})
    return boxes


def build_bank(names: list[str], per_class: int) -> Bank:
    """Simulate a user labeling `per_class` images for each class, cheapest
    first: images are taken in filename order, exactly like the pool listing.
    One embedding per (image, class), which is what POST /api/label does."""
    if SCRATCH.exists():
        shutil.rmtree(SCRATCH)
    bank = Bank(str(SCRATCH))
    taught = {n: 0 for n in names}
    sizes = {n: [] for n in names}

    for img_path in sorted(POOL.glob("*.jpg")):
        txt = POOL_GT / f"{img_path.stem}.txt"
        if not txt.exists() or all(v >= per_class for v in taught.values()):
            continue
        img = cv2.imread(str(img_path))
        if img is None:
            continue
        h, w = img.shape[:2]
        by_class: dict[str, list[list[float]]] = {}
        for b in read_yolo(txt, w, h, names):
            by_class.setdefault(b["cls"], []).append(b["box"])
            x1, y1, x2, y2 = b["box"]
            sizes[b["cls"]].append((x2 - x1) * (y2 - y1) / (w * h) * 100)
        for cls_name, boxes in by_class.items():
            if taught[cls_name] >= per_class:
                continue
            bank.add(cls_name, extract_embedding(img, boxes), str(img_path), boxes[0])
            taught[cls_name] += 1

    print("prompt bank:", {n: bank.count(n) for n in bank.classes})
    for n, vals in sizes.items():
        if vals:
            print(f"  {n}: median box = {statistics.median(vals):.2f}% of the image "
                  f"({len(vals)} boxes seen while building)")
    return bank


def main():
    per_class = int(sys.argv[1]) if len(sys.argv) > 1 else 20
    confs = [float(a) for a in sys.argv[2:]] or DEFAULT_CONFS
    names = [n for n in (TEST / "classes.txt").read_text(encoding="utf-8").splitlines() if n.strip()]
    gt = metrics.load_ground_truth(str(TEST))
    print(f"test set: {len(gt)} images, "
          f"{sum(len(v) for v in gt.values())} ground-truth boxes, classes {names}")

    bank = build_bank(names, per_class)
    bank_names, combined = bank.mean_vpe()
    model = arm(bank_names, combined)

    rows = []
    for conf in confs:
        pred = {p: predict_one(model, bank_names, p, conf) for p in gt}
        res = metrics.evaluate(gt, pred)
        rows.append((conf, res))
        print(f"conf={conf:.2f} done — overall F1 {res['overall']['f1']:.3f}")

    empty = {"precision": 0, "recall": 0, "f1": 0, "tp": 0, "fp": 0, "fn": 0}

    def table(label, res):
        for name in bank_names + ["(overall)"]:
            m = res["overall"] if name == "(overall)" else res["per_class"].get(name, empty)
            print(f"| {label} | {name} | {m['precision']:.3f} | {m['recall']:.3f} "
                  f"| {m['f1']:.3f} | {m['tp']} | {m['fp']} | {m['fn']} |")

    print("\n| conf | class | precision | recall | F1 | TP | FP | FN |")
    print("|---|---|---|---|---|---|---|---|")
    for conf, res in rows:
        table(f"{conf:.2f}", res)

    # The reason the table matters: each class peaks at a different threshold,
    # so one global conf always sacrifices one of them. conf_by_class gives
    # both their best in a single pass -- this row is the proof it does.
    best = {n: max(rows, key=lambda r: r[1]["per_class"].get(n, empty)["f1"])[0]
            for n in bank_names}
    pred = {p: predict_one(model, bank_names, p, max(best.values()), best) for p in gt}
    table("per-class " + ", ".join(f"{n}={v:.2f}" for n, v in best.items()),
          metrics.evaluate(gt, pred))

    shutil.rmtree(SCRATCH)


if __name__ == "__main__":
    main()
