"""T-24 -- download this project's annotations in whichever format a
training pipeline actually wants. Reads straight out of PostgreSQL
(services/annotations_db.py); nothing here writes anything, and it doesn't
care whether the images came from the pool or the held-out test set.

Only three formats -- YOLO (what this tool always produced), COCO, and
Pascal VOC -- cover every consumer anyone's asked for so far (see
docs/DB_MIGRATION_PLAN.md #6). Add a fourth _export_x() the day someone
actually needs one; the dispatch below is a plain dict, not a registry."""
import io
import json
import zipfile
from pathlib import Path
from xml.sax.saxutils import escape

import cv2
from fastapi import APIRouter, HTTPException, Response

from ..deps import checked_path
from ..services import annotations_db

router = APIRouter(prefix="/api", tags=["export"])


def _dims(path: str) -> tuple[int, int] | None:
    """(width, height), or None if the file has moved/been deleted since it
    was annotated -- that image is skipped rather than failing the whole
    export."""
    img = cv2.imread(path)
    if img is None:
        return None
    h, w = img.shape[:2]
    return w, h


def _export_yolo(names: list[str], by_image: dict[str, list[dict]]) -> bytes:
    idx = {n: i for i, n in enumerate(names)}
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as zf:
        zf.writestr("classes.txt", "\n".join(names))
        for path, boxes in by_image.items():
            dims = _dims(path)
            if dims is None:
                continue
            w, h = dims
            lines = []
            for b in boxes:
                x1, y1, x2, y2 = b["box"]
                cx, cy = (x1 + x2) / 2 / w, (y1 + y2) / 2 / h
                bw, bh = abs(x2 - x1) / w, abs(y2 - y1) / h
                lines.append(f"{idx[b['cls']]} {cx:.6f} {cy:.6f} {bw:.6f} {bh:.6f}")
            zf.writestr(f"labels/{Path(path).stem}.txt", "\n".join(lines))
    return buf.getvalue()


def _export_coco(names: list[str], by_image: dict[str, list[dict]]) -> bytes:
    cat_id = {n: i + 1 for i, n in enumerate(names)}
    categories = [{"id": cid, "name": n} for n, cid in cat_id.items()]
    images, annotations = [], []
    ann_id = 1
    for image_id, (path, boxes) in enumerate(by_image.items(), start=1):
        dims = _dims(path)
        if dims is None:
            continue
        w, h = dims
        images.append({"id": image_id, "file_name": Path(path).name, "width": w, "height": h})
        for b in boxes:
            x1, y1, x2, y2 = b["box"]
            bw, bh = x2 - x1, y2 - y1
            annotations.append({
                "id": ann_id, "image_id": image_id, "category_id": cat_id[b["cls"]],
                "bbox": [x1, y1, bw, bh], "area": bw * bh, "iscrowd": 0,
            })
            ann_id += 1
    return json.dumps(
        {"images": images, "annotations": annotations, "categories": categories},
        ensure_ascii=False,
    ).encode("utf-8")


def _export_voc(names: list[str], by_image: dict[str, list[dict]]) -> bytes:
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as zf:
        for path, boxes in by_image.items():
            dims = _dims(path)
            if dims is None:
                continue
            w, h = dims
            objects = "".join(
                f"<object><name>{escape(b['cls'])}</name><bndbox>"
                f"<xmin>{b['box'][0]:.1f}</xmin><ymin>{b['box'][1]:.1f}</ymin>"
                f"<xmax>{b['box'][2]:.1f}</xmax><ymax>{b['box'][3]:.1f}</ymax>"
                "</bndbox></object>"
                for b in boxes
            )
            xml = (
                f"<annotation><filename>{escape(Path(path).name)}</filename>"
                f"<size><width>{w}</width><height>{h}</height><depth>3</depth></size>"
                f"{objects}</annotation>"
            )
            zf.writestr(f"{Path(path).stem}.xml", xml)
    return buf.getvalue()


_EXPORTERS = {
    "yolo": ("application/zip", "labels_yolo.zip", _export_yolo),
    "coco": ("application/json", "annotations_coco.json", _export_coco),
    "voc": ("application/zip", "labels_voc.zip", _export_voc),
}


@router.get("/export")
def export(input_dir: str, format: str = "yolo", kind: str = "pool"):
    if format not in _EXPORTERS:
        raise HTTPException(400, f"unknown format {format!r} -- choose one of {sorted(_EXPORTERS)}")
    if kind not in ("pool", "testset"):
        raise HTTPException(400, f"unknown kind {kind!r} -- choose 'pool' or 'testset'")
    inp = str(checked_path(input_dir))
    names = annotations_db.get_classes(inp, kind)
    by_image = annotations_db.load_annotations(inp, kind)
    if not names or not by_image:
        raise HTTPException(400, f"nothing to export for {kind!r} -- label something first")

    media, filename, build = _EXPORTERS[format]
    return Response(
        content=build(names, by_image), media_type=media,
        headers={"Content-Disposition": f'attachment; filename="{filename}"'},
    )
