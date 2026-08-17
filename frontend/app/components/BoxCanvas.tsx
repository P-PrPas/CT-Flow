"use client";

import { useEffect, useRef, useState } from "react";

export type Rect = [number, number, number, number];
export type Box = { cls: string; box: Rect };

/** Which corner of a box a pointer grabbed: [moves x1?, moves y1?]. */
const CORNERS: [number, number][] = [[0, 0], [1, 0], [0, 1], [1, 1]];
/** Grab radius for a corner handle, in displayed pixels. */
const GRAB = 12;

/** Keep receiving move/up after the pointer leaves the image. Best-effort:
 *  losing capture costs a drag that stops at the edge, never the drag itself. */
const capture = (e: React.PointerEvent) => {
  try { (e.currentTarget as Element).setPointerCapture(e.pointerId); } catch { /* no-op */ }
};

/** Drag on empty space to draw a box. Click a box to select it, then drag it to
 *  move or drag a corner to resize -- a model's guess is usually nearly right,
 *  and "delete it and draw it again" throws that away. Delete removes the
 *  selection (or hit the ×). Coordinates live in the source image's pixel
 *  space, so they survive any display scaling. */
export default function BoxCanvas({
  src,
  boxes,
  color,
  onAdd,
  onRemove,
  onUpdate,
  savedBoxes = [],
  draftBoxes = [],
  onRemoveDraft,
  selected,
  onSelect,
}: {
  src: string;
  boxes: Box[];
  color: (cls: string) => string;
  onAdd: (b: Rect) => void;
  /** Delete boxes[index]. Omit to make `boxes` non-interactive. */
  onRemove?: (index: number) => void;
  /** Move/resize boxes[index]. Called once when the drag ends, so one edit is
   *  one undo step. Omit to make boxes fixed once drawn. */
  onUpdate?: (index: number, b: Rect) => void;
  /** Already-saved boxes, dimmed and inert, so a labeled image never looks blank. */
  savedBoxes?: Box[];
  /** FR-19 pre-annotation: model guesses awaiting confirmation. Deliberately
   *  drawn unlike anything the user drew -- an unconfirmed guess must never be
   *  mistaken for a decision someone made. */
  draftBoxes?: Box[];
  /** Drop draftBoxes[index] -- a false positive the user prunes before
   *  accepting the rest. Omit to make drafts un-selectable (old behavior). */
  onRemoveDraft?: (index: number) => void;
  selected?: number | null;
  onSelect?: (i: number | null) => void;
}) {
  const imgRef = useRef<HTMLImageElement>(null);
  const [drag, setDrag] = useState<{ x0: number; y0: number; x1: number; y1: number } | null>(null);
  /** An in-progress move/resize. Held locally and pushed up once on release,
   *  so dragging a corner is a single undo step rather than sixty. */
  const [live, setLive] = useState<{ i: number; box: Rect; grab: [number, number] | null; from: { x: number; y: number } } | null>(null);
  const [size, setSize] = useState({ w: 1, h: 1 });
  const [ownSelected, setOwnSelected] = useState<number | null>(null);
  /** Drafts have their own selection -- they live in a separate array from
   *  `boxes`, so one index space can't cover both. */
  const [draftSel, setDraftSel] = useState<number | null>(null);

  const sel = selected !== undefined ? selected : ownSelected;
  const setSel = (i: number | null) => (onSelect ? onSelect(i) : setOwnSelected(i));

  // A new image loaded -> the old index would point at the wrong box.
  useEffect(() => { setSel(null); setDraftSel(null); /* eslint-disable-next-line react-hooks/exhaustive-deps */ }, [src]);

  const hitTestDrafts = (x: number, y: number) => {
    for (let i = draftBoxes.length - 1; i >= 0; i--) {
      const [x1, y1, x2, y2] = draftBoxes[i].box;
      if (x >= x1 && x <= x2 && y >= y1 && y <= y2) return i;
    }
    return -1;
  };

  /** Source-image pixels per displayed pixel -- the grab radius has to be
   *  constant on screen, not in the image's coordinate space. */
  const scale = () => {
    const r = imgRef.current?.getBoundingClientRect();
    return r && r.width ? size.w / r.width : 1;
  };

  const at = (e: React.PointerEvent) => {
    const r = imgRef.current!.getBoundingClientRect();
    return {
      x: ((e.clientX - r.left) / r.width) * size.w,
      y: ((e.clientY - r.top) / r.height) * size.h,
    };
  };

  const hitTest = (x: number, y: number) => {
    for (let i = boxes.length - 1; i >= 0; i--) {
      const [x1, y1, x2, y2] = boxes[i].box;
      if (x >= x1 && x <= x2 && y >= y1 && y <= y2) return i;
    }
    return -1;
  };

  /** Which corner of the selected box is under the pointer, if any. */
  const grabbedCorner = (x: number, y: number): [number, number] | null => {
    if (sel === null || !boxes[sel]) return null;
    const [x1, y1, x2, y2] = boxes[sel].box;
    const near = GRAB * scale();
    for (const [gx, gy] of CORNERS) {
      const cx = gx ? x1 : x2;
      const cy = gy ? y1 : y2;
      if (Math.abs(x - cx) <= near && Math.abs(y - cy) <= near) return [gx, gy];
    }
    return null;
  };

  /** Normalized so a corner dragged past its opposite still gives x1<x2. */
  const norm = (b: Rect): Rect =>
    [Math.min(b[0], b[2]), Math.min(b[1], b[3]), Math.max(b[0], b[2]), Math.max(b[1], b[3])];

  const shown = (i: number) => (live && live.i === i ? norm(live.box) : boxes[i].box);

  /** Where an in-progress move/resize puts its box, given the pointer at `p`.
   *  Anchored on the box as it was when the drag started, so it is the same
   *  answer whether it is asked during the drag or at the end of it. */
  const dragged = (l: NonNullable<typeof live>, p: { x: number; y: number }): Rect => {
    const [x1, y1, x2, y2] = boxes[l.i].box;
    if (l.grab) {
      return [l.grab[0] ? p.x : x1, l.grab[1] ? p.y : y1,
              l.grab[0] ? x2 : p.x, l.grab[1] ? y2 : p.y];
    }
    const dx = p.x - l.from.x, dy = p.y - l.from.y;
    return [x1 + dx, y1 + dy, x2 + dx, y2 + dy];
  };

  const rect = (b: number[]) => ({
    left: `${(b[0] / size.w) * 100}%`,
    top: `${(b[1] / size.h) * 100}%`,
    width: `${((b[2] - b[0]) / size.w) * 100}%`,
    height: `${((b[3] - b[1]) / size.h) * 100}%`,
  });

  return (
    <div
      className="canvas-wrap"
      style={{ userSelect: "none", touchAction: "none", cursor: "crosshair" }}
      onPointerDown={(e) => {
        const p = at(e);
        if (onUpdate) {
          // A grab on the selected box's corner resizes; anywhere inside a box
          // moves it. Both beat starting a new box on top of an existing one.
          const corner = grabbedCorner(p.x, p.y);
          const hit = corner ? sel! : hitTest(p.x, p.y);
          if (hit >= 0) {
            setSel(hit);
            setLive({ i: hit, box: [...boxes[hit].box], grab: corner, from: p });
            capture(e);
            return;
          }
        } else if (onRemove) {
          const hit = hitTest(p.x, p.y);
          if (hit >= 0) { setSel(hit); setDraftSel(null); return; }
        }
        if (onRemoveDraft) {
          const hit = hitTestDrafts(p.x, p.y);
          if (hit >= 0) { setDraftSel(hit); setSel(null); return; }
        }
        setSel(null);
        setDraftSel(null);
        setDrag({ x0: p.x, y0: p.y, x1: p.x, y1: p.y });
        capture(e);
      }}
      onPointerMove={(e) => {
        const p = at(e);
        if (live) { setLive({ ...live, box: dragged(live, p) }); return; }
        if (!drag) return;
        setDrag({ ...drag, x1: p.x, y1: p.y });
      }}
      onPointerUp={(e) => {
        // Both branches recompute from THIS event rather than trusting the last
        // pointermove: move events are batched, so a fast drag can land here
        // with the state still at the position the drag started from.
        const p = at(e);
        if (live) {
          const b = norm(dragged(live, p));
          setLive(null);
          // A click that never moved is a selection, not an edit -- don't burn
          // an undo step on it.
          if (b.some((v, i) => Math.abs(v - boxes[live.i].box[i]) > 0.5)) onUpdate?.(live.i, b);
          return;
        }
        if (!drag) return;
        const b = norm([drag.x0, drag.y0, p.x, p.y]);
        setDrag(null);
        if (b[2] - b[0] > 4 && b[3] - b[1] > 4) onAdd(b); // ignore stray clicks
      }}
    >
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img
        ref={imgRef}
        src={src}
        alt="Image being labeled"
        draggable={false}
        onLoad={(e) => setSize({ w: e.currentTarget.naturalWidth, h: e.currentTarget.naturalHeight })}
        style={{ width: "100%", display: "block" }}
      />

      {savedBoxes.map((b, i) => (
        <div
          key={`s${i}`}
          style={{
            position: "absolute", ...rect(b.box),
            border: `2px dashed ${color(b.cls)}`,
            opacity: .5, pointerEvents: "none", borderRadius: 2,
          }}
        >
          <span className="box-label" style={{ bottom: -18, color: color(b.cls), background: "rgba(4,8,16,.78)" }}>
            {b.cls} · saved
          </span>
        </div>
      ))}

      {draftBoxes.map((b, i) => (
        <div
          key={`d${i}`}
          style={{
            position: "absolute", ...rect(b.box),
            border: `2px dotted ${color(b.cls)}`,
            background: i === draftSel ? `${color(b.cls)}30` : `${color(b.cls)}14`,
            boxShadow: i === draftSel ? `0 0 0 2px ${color(b.cls)}55` : undefined,
            opacity: .75, pointerEvents: "none", borderRadius: 2,
          }}
        >
          <span className="box-label" style={{ top: -18, color: "#04262e", background: color(b.cls) }}>
            {b.cls} · suggested
          </span>
          {onRemoveDraft && i === draftSel && (
            <button
              onPointerDown={(e) => e.stopPropagation()}
              onClick={(e) => { e.stopPropagation(); onRemoveDraft(i); setDraftSel(null); }}
              title="Discard this suggestion (Delete)"
              aria-label="Discard this suggested box"
              style={{
                position: "absolute", top: -28, right: -8, width: 22, height: 22,
                borderRadius: "50%", background: "var(--bad)", color: "#fff",
                border: "2px solid #0b1120", cursor: "pointer", fontSize: 13,
                lineHeight: "16px", padding: 0, pointerEvents: "auto",
                display: "grid", placeItems: "center",
              }}
            >
              ×
            </button>
          )}
        </div>
      ))}

      {boxes.map((b, i) => (
        <div
          key={i}
          style={{
            position: "absolute", ...rect(shown(i)),
            border: `2px solid ${color(b.cls)}`,
            background: i === sel ? `${color(b.cls)}1f` : "transparent",
            boxShadow: i === sel ? `0 0 0 2px ${color(b.cls)}55` : undefined,
            pointerEvents: "none", borderRadius: 2,
          }}
        >
          <span className="box-label" style={{ top: -18, color: "#04262e", background: color(b.cls) }}>
            {b.cls}
          </span>
          {onUpdate && i === sel && CORNERS.map(([gx, gy]) => (
            <span
              key={`${gx}${gy}`}
              style={{
                position: "absolute", width: 9, height: 9, borderRadius: 2,
                background: color(b.cls), border: "1.5px solid #0b1120",
                [gx ? "left" : "right"]: -5,
                [gy ? "top" : "bottom"]: -5,
                pointerEvents: "none",
              }}
            />
          ))}
          {onRemove && i === sel && (
            <button
              onPointerDown={(e) => e.stopPropagation()}
              onClick={(e) => { e.stopPropagation(); onRemove(i); setSel(null); }}
              title="Remove this box (Delete)"
              aria-label="Remove this box"
              style={{
                // Clear of the top-right resize handle -- one of them has to
                // move, and deleting by accident is the worse mistake.
                position: "absolute", top: -28, right: -8, width: 22, height: 22,
                borderRadius: "50%", background: "var(--bad)", color: "#fff",
                border: "2px solid #0b1120", cursor: "pointer", fontSize: 13,
                lineHeight: "16px", padding: 0, pointerEvents: "auto",
                display: "grid", placeItems: "center",
              }}
            >
              ×
            </button>
          )}
        </div>
      ))}

      {drag && (
        <div
          style={{
            position: "absolute",
            ...rect([
              Math.min(drag.x0, drag.x1), Math.min(drag.y0, drag.y1),
              Math.max(drag.x0, drag.x1), Math.max(drag.y0, drag.y1),
            ]),
            border: "2px dashed var(--brand)",
            background: "rgba(8,217,214,.12)",
            pointerEvents: "none", borderRadius: 2,
          }}
        />
      )}
    </div>
  );
}
