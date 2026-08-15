"""Read/write one image's YOLO label file (labels/<stem>.txt against a
classes.txt index). Shared by bank.py (a pool's output_dir) and
groundtruth.py (a test_dir) -- both use this exact layout, they just grow
their class list differently.

`names` must be the same ordered list used when the file was last written --
class order has to be append-only and never re-sorted, or old files decode
under the wrong index (see bank.py's `classes` property).
"""
from pathlib import Path

import cv2


def read_boxes(dir_path: str, image_path: str, names: list[str]) -> list[dict]:
    """[{"cls": name, "box": [x1,y1,x2,y2]}] in pixel coords, or [] if the
    image has no label file yet."""
    txt = Path(dir_path) / "labels" / (Path(image_path).stem + ".txt")
    if not txt.exists():
        return []
    img = cv2.imread(image_path)
    if img is None:
        return []
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
    return boxes


def write_boxes(dir_path: str, image_path: str, boxes: list[dict],
                 width: int, height: int, names: list[str]) -> None:
    """boxes: [{"cls": name, "box": [x1,y1,x2,y2]}] in pixel coords. Fully
    replaces the image's label file with exactly this set -- callers that
    want to preserve what's already there merge it in first via
    read_boxes()."""
    d = Path(dir_path)
    (d / "labels").mkdir(parents=True, exist_ok=True)
    idx = {n: i for i, n in enumerate(names)}
    lines = []
    for b in boxes:
        x1, y1, x2, y2 = b["box"]
        cx, cy = (x1 + x2) / 2 / width, (y1 + y2) / 2 / height
        bw, bh = abs(x2 - x1) / width, abs(y2 - y1) / height
        lines.append(f"{idx.get(b['cls'], 0)} {cx:.6f} {cy:.6f} {bw:.6f} {bh:.6f}")
    (d / "labels" / (Path(image_path).stem + ".txt")).write_text(
        "\n".join(lines), encoding="utf-8"
    )
