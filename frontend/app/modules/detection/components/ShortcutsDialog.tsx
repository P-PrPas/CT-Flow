"use client";

import Modal from "../../../components/Modal";
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
    group: "Drawing with the mouse",
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
    group: "Drawing from the keyboard",
    items: [
      ["Tab", "Focus the image, then step through its boxes"],
      ["Arrows", "Move the crosshair, or the selected box"],
      ["Shift + Arrows", "The same, ten times faster"],
      ["Enter", "Drop the first corner, then the second"],
      ["Alt + Arrows", "Resize the selected box"],
      ["Delete", "Remove the selected box"],
      ["Esc", "Cancel the box being drawn, or deselect"],
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

export default function ShortcutsDialog({
  enabled, onToggle, onClose,
}: { enabled: boolean; onToggle: (on: boolean) => void; onClose: () => void }) {
  return (
    <Modal label="Keyboard shortcuts" width={560} onClose={onClose}>
      <div className="modal-head">
        <h2 className="row" style={{ gap: 8, fontSize: 14 }}>
          <span style={{ color: "var(--brand)" }}><Icon name="keyboard" size={16} /></span>
          Keyboard shortcuts
        </h2>
        <button className="btn ghost icon" onClick={onClose} aria-label="Close">
          <Icon name="x" size={15} />
        </button>
      </div>

      <div className="modal-body col" style={{ gap: 18 }}>
        {/* 2.1.4 — single-character shortcuts have to be turnable off, or
            anyone dictating into this page fires them by talking. */}
        <label className="check note info" style={{ alignItems: "flex-start" }}>
          <input type="checkbox" checked={enabled} onChange={(e) => onToggle(e.target.checked)} />
          <span className="col" style={{ gap: 2 }}>
            <strong style={{ color: "var(--text)" }}>
              Single-key shortcuts are {enabled ? "on" : "off"}
            </strong>
            <span className="xs">
              Turn them off if you use speech input — otherwise dictating a word with an
              &ldquo;s&rdquo; in it skips an image. Ctrl-key shortcuts keep working either way,
              and so does everything on the image itself.
            </span>
          </span>
        </label>

        {SHORTCUTS.map((g) => (
          <div key={g.group} className="col" style={{ gap: 7 }}>
            <h3 className="card-title">{g.group}</h3>
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
    </Modal>
  );
}
