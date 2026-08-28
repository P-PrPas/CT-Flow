"use client";

import { useEffect } from "react";
import { Icon } from "../../../lib/ui";

/** FR-20 — the shortcut sheet, opened with `?`. Every repeated action in the
 *  labeling loop has a key, so a full image can be labeled without the mouse
 *  leaving the canvas. */
export const SHORTCUTS: { group: string; items: [string, string][] }[] = [
  {
    group: "Moving through images",
    items: [
      ["→ / N", "Next image"],
      ["← / P", "Previous image"],
      ["S", "Skip this image"],
    ],
  },
  {
    group: "Drawing",
    items: [
      ["Drag", "Draw a box"],
      ["A", "Take the model's suggested boxes"],
      ["1 – 9", "Pick the class for the next box"],
      ["Click box", "Select it"],
      ["Delete / Backspace", "Remove the selected box"],
      ["Ctrl + Z", "Undo"],
      ["Ctrl + Shift + Z", "Redo"],
      ["C", "Copy boxes from the last image you saved"],
      ["Esc", "Clear the current boxes"],
    ],
  },
  {
    group: "Saving",
    items: [
      ["Enter", "Save and move to the next image"],
      ["Ctrl + S", "Save (teach the model, or save corrections in review)"],
      ["?", "Open this list"],
    ],
  },
];

export default function ShortcutsDialog({ onClose }: { onClose: () => void }) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") onClose(); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div className="scrim" onClick={onClose} role="dialog" aria-modal="true" aria-label="Keyboard shortcuts">
      <div className="modal" onClick={(e) => e.stopPropagation()} style={{ maxWidth: 560 }}>
        <div className="modal-head">
          <strong className="row" style={{ gap: 8, fontSize: 14 }}>
            <span style={{ color: "var(--brand)" }}><Icon name="keyboard" size={16} /></span>
            Keyboard shortcuts
          </strong>
          <button className="btn ghost icon" onClick={onClose} aria-label="Close">
            <Icon name="x" size={15} />
          </button>
        </div>
        <div className="modal-body col" style={{ gap: 18 }}>
          {SHORTCUTS.map((g) => (
            <div key={g.group} className="col" style={{ gap: 7 }}>
              <div className="card-title">{g.group}</div>
              {g.items.map(([key, what]) => (
                <div key={key} className="row between" style={{ gap: 16 }}>
                  <span className="sm">{what}</span>
                  <span className="row" style={{ gap: 4 }}>
                    {key.split(" ").map((k, i) =>
                      k === "+" || k === "/" || k === "–"
                        ? <span key={i} className="faint xs">{k}</span>
                        : <kbd key={i}>{k}</kbd>
                    )}
                  </span>
                </div>
              ))}
            </div>
          ))}
        </div>
        <div className="modal-foot">
          <span className="xs muted">Shortcuts pause while you are typing in a field.</span>
          <button className="btn primary" onClick={onClose}>Got it</button>
        </div>
      </div>
    </div>
  );
}
