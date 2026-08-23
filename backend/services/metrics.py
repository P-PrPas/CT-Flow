"""Accuracy of the current prompt bank against a hand-labeled test set.

The test set is whichever pool images are flagged in <test_dir>/testset.json
(see services/groundtruth.py) -- ground truth is written in the same shape
this tool always uses:

    <test_dir>/labels/*.txt     YOLO ground truth
    <test_dir>/classes.txt

Metric is precision / recall / F1 at IoU 0.5 with greedy one-to-one matching.
ponytail: no mAP -- P/R/F1 at a fixed threshold is what you actually decide
"is it good enough to auto-label" on. Add mAP50 if someone wants to compare
against published numbers.
"""
from pathlib import Path

import cv2

from . import groundtruth


def iou(a: list[float], b: list[float]) -> float:
    ix1, iy1 = max(a[0], b[0]), max(a[1], b[1])
    ix2, iy2 = min(a[2], b[2]), min(a[3], b[3])
    inter = max(0.0, ix2 - ix1) * max(0.0, iy2 - iy1)
    if inter <= 0:
        return 0.0
    area_a = (a[2] - a[0]) * (a[3] - a[1])
    area_b = (b[2] - b[0]) * (b[3] - b[1])
    return inter / (area_a + area_b - inter)


def load_ground_truth(test_dir: str) -> dict[str, list[dict]]:
    """{image_path: [{"cls": name, "box": [x1,y1,x2,y2]}]} in pixel coords."""
    d = Path(test_dir)
    classes_file = d / "classes.txt"
    if not classes_file.exists():
        raise FileNotFoundError(f"{classes_file} missing -- label the test set first")
    names = [n for n in classes_file.read_text(encoding="utf-8").splitlines() if n.strip()]

    gt: dict[str, list[dict]] = {}
    for img_path in (Path(p) for p in groundtruth.list_test_images(test_dir)):
        txt = d / "labels" / (img_path.stem + ".txt")
        if not txt.exists():
            continue  # flagged but not yet labeled -> not part of the test set
        if not img_path.is_file():
            continue  # the pool image this manifest entry pointed at moved or was deleted
        img = cv2.imread(str(img_path))
        if img is None:
            continue
        h, w = img.shape[:2]
        boxes = []
        for line in txt.read_text(encoding="utf-8").splitlines():
            parts = line.split()
            if len(parts) != 5:
                continue
            k, cx, cy, bw, bh = int(parts[0]), *(float(v) for v in parts[1:])
            boxes.append({
                "cls": names[k] if k < len(names) else str(k),
                "box": [(cx - bw / 2) * w, (cy - bh / 2) * h,
                        (cx + bw / 2) * w, (cy + bh / 2) * h],
            })
        gt[str(img_path)] = boxes
    if not gt:
        raise FileNotFoundError(f"no labeled images found in {test_dir}")
    return gt


def evaluate(gt: dict[str, list[dict]], pred: dict[str, list[dict]],
             iou_thr: float = 0.5) -> dict:
    per_class: dict[str, dict[str, int]] = {}

    def bucket(name: str) -> dict[str, int]:
        return per_class.setdefault(name, {"tp": 0, "fp": 0, "fn": 0})

    per_image = []
    for path, truths in gt.items():
        dets = sorted(pred.get(path, []), key=lambda d: -d["conf"])
        taken = set()
        gt_out = [dict(t, matched=False) for t in truths]
        pred_out = []
        for d in dets:
            best, best_iou = None, iou_thr
            for i, t in enumerate(truths):
                if i in taken or t["cls"] != d["cls"]:
                    continue
                v = iou(d["box"], t["box"])
                if v >= best_iou:
                    best, best_iou = i, v
            matched = best is not None
            if matched:
                taken.add(best)
                gt_out[best]["matched"] = True
                bucket(d["cls"])["tp"] += 1
            else:
                bucket(d["cls"])["fp"] += 1
            pred_out.append({"cls": d["cls"], "box": d["box"], "conf": d["conf"], "matched": matched})
        for i, t in enumerate(truths):
            if i not in taken:
                bucket(t["cls"])["fn"] += 1
        per_image.append({
            "image": path, "gt": gt_out, "pred": pred_out,
            "tp": sum(1 for p in pred_out if p["matched"]),
            "fp": sum(1 for p in pred_out if not p["matched"]),
            "fn": sum(1 for g in gt_out if not g["matched"]),
        })

    tp = sum(c["tp"] for c in per_class.values())
    fp = sum(c["fp"] for c in per_class.values())
    fn = sum(c["fn"] for c in per_class.values())

    def prf(tp: int, fp: int, fn: int) -> dict:
        p = tp / (tp + fp) if tp + fp else 0.0
        r = tp / (tp + fn) if tp + fn else 0.0
        f = 2 * p * r / (p + r) if p + r else 0.0
        return {"precision": p, "recall": r, "f1": f, "tp": tp, "fp": fp, "fn": fn}

    return {
        "overall": prf(tp, fp, fn),
        "per_class": {n: prf(c["tp"], c["fp"], c["fn"]) for n, c in sorted(per_class.items())},
        "per_image": per_image,
        "images": len(gt),
        "iou": iou_thr,
    }


def demo():
    """Self-check: one perfect match, one miss, one spurious box."""
    gt = {"a": [{"cls": "x", "box": [0, 0, 10, 10]}, {"cls": "x", "box": [50, 50, 60, 60]}]}
    pred = {"a": [{"cls": "x", "box": [0, 0, 10, 9], "conf": 0.9},
                  {"cls": "x", "box": [90, 90, 99, 99], "conf": 0.4}]}
    full = evaluate(gt, pred)
    r = full["overall"]
    assert (r["tp"], r["fp"], r["fn"]) == (1, 1, 1), r
    assert abs(r["precision"] - 0.5) < 1e-9 and abs(r["recall"] - 0.5) < 1e-9
    assert iou([0, 0, 10, 10], [0, 0, 10, 10]) == 1.0
    assert iou([0, 0, 1, 1], [5, 5, 6, 6]) == 0.0
    img = full["per_image"][0]
    assert [g["matched"] for g in img["gt"]] == [True, False]
    assert [p["matched"] for p in img["pred"]] == [True, False]
    print("metrics demo OK")


if __name__ == "__main__":
    demo()
