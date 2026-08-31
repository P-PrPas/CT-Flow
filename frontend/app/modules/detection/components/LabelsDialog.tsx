"use client";

import { useState } from "react";
import type { FormEvent } from "react";
import Modal from "../../../components/Modal";
import type { Session } from "../session";
import { Icon } from "../../../lib/ui";

/** Set up the label list before drawing anything.
 *
 *  A new project starts with no classes at all, and the only way to make one
 *  used to be typing into a 128px box mid-draw -- which is a bad moment to be
 *  deciding what the classes are.
 *
 *  Two kinds of row, and the difference matters:
 *
 *  - **Taught.** The bank holds examples under this name, so the server has a
 *    class row for it and every box saved under it refers to it by position.
 *    Class indexes are append-only (CLAUDE.md invariant 1), so there is no
 *    safe delete here and the button says so rather than being quietly absent.
 *  - **Planned.** A name and nothing else. Removing one deletes nothing,
 *    because nothing was ever created.
 *
 *  Colours come from the server's palette rather than a free picker: those
 *  values were chosen to stay separable under colour-vision deficiency and to
 *  clear 3:1 on the dark canvas, and a free picker is how a class ends up
 *  #333 on #111. */
export default function LabelsDialog({ s, onClose }: { s: Session; onClose: () => void }) {
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  /** Which row has its palette open. One at a time -- twelve swatches per row
   *  times eight rows is a wall, not a choice. */
  const [editing, setEditing] = useState<string | null>(null);

  const taught = new Map((s.bank?.classes ?? []).map((c) => [c.name, c.count] as const));

  const add = (e: FormEvent) => {
    e.preventDefault();
    const n = name.trim();
    if (!n) return;
    if (s.classNames.some((c) => c.toLowerCase() === n.toLowerCase())) {
      setError(`“${n}” is already on the list.`);
      return;
    }
    s.addClass(n);
    // The first label someone adds is almost certainly the one they are about
    // to draw, and an empty class box is the thing that blocks saving.
    if (!s.cls.trim()) s.setCls(n);
    setName("");
    setError("");
  };

  return (
    <Modal label="Labels for this project" width={560} onClose={onClose}>
      <div className="modal-head">
        <h2 className="card-title"><Icon name="sliders" size={13} /> Labels</h2>
        <button className="btn ghost icon" onClick={onClose} aria-label="Close">
          <Icon name="x" size={15} />
        </button>
      </div>

      <div className="modal-body col" style={{ gap: 14 }}>
        <form className="col" style={{ gap: 6 }} onSubmit={add}>
          <label className="xs muted" htmlFor="new-label">Add a label</label>
          <div className="row" style={{ gap: 6 }}>
            <input
              id="new-label"
              className="grow"
              autoFocus
              value={name}
              placeholder="what you are looking for — “scratch”, “missing screw”…"
              aria-describedby={error ? "new-label-error" : undefined}
              onChange={(e) => { setName(e.target.value); setError(""); }}
            />
            <button className="btn primary" disabled={!name.trim()}>
              <Icon name="plus" size={14} /> Add
            </button>
          </div>
          {error && (
            <span className="xs" id="new-label-error" role="alert" style={{ color: "var(--bad)" }}>
              {error}
            </span>
          )}
        </form>

        {s.classNames.length === 0 ? (
          <div className="note info">
            <Icon name="info" size={15} />
            <span>
              No labels yet. Add one for each kind of thing you want found — you can add
              more later, and the first box you draw is what actually teaches the model.
            </span>
          </div>
        ) : (
          <div className="col" style={{ gap: 2 }}>
            {s.classNames.map((n) => {
              const count = taught.get(n);
              const open = editing === n;
              return (
                <div key={n} className="col" style={{ gap: 6, padding: "7px 2px", borderTop: "1px solid var(--line-soft)" }}>
                  <div className="row" style={{ gap: 8 }}>
                    <button
                      type="button"
                      className="btn sm"
                      aria-expanded={open}
                      aria-label={`Colour for ${n}`}
                      onClick={() => setEditing(open ? null : n)}
                    >
                      <span className="dot" style={{ color: s.color(n), width: 11, height: 11 }} />
                      <Icon name={open ? "chevronDown" : "chevronRight"} size={12} />
                    </button>

                    <span className="grow truncate" style={{ fontWeight: 500 }}>{n}</span>

                    {count === undefined ? (
                      <span className="chip">not drawn yet</span>
                    ) : (
                      <span className="chip ok">
                        <Icon name="brain" size={12} /> {count} example{count === 1 ? "" : "s"}
                      </span>
                    )}

                    <button
                      type="button"
                      className="btn sm danger"
                      disabled={count !== undefined}
                      title={
                        count === undefined
                          ? `Remove ${n} from the list`
                          : "Already taught — every box saved under this name points at it, so it cannot be removed"
                      }
                      onClick={() => { s.removeClass(n); if (editing === n) setEditing(null); }}
                    >
                      <Icon name="trash" size={13} />
                      <span className="sr-only">Remove {n}</span>
                    </button>
                  </div>

                  {open && (
                    <div className="col" style={{ gap: 6, paddingLeft: 4 }}>
                      <div className="palette">
                        {s.colors.map((hex, i) => {
                          const on = s.color(n) === hex;
                          return (
                            <button
                              type="button"
                              key={hex}
                              aria-pressed={on}
                              aria-label={`Colour ${i + 1} for ${n}`}
                              style={{ background: hex }}
                              onClick={() => s.setClassColor(n, hex)}
                            >
                              {/* A check, not just a ring: which one is chosen
                                  must not be carried by colour alone. */}
                              {on && <Icon name="check" size={13} />}
                            </button>
                          );
                        })}
                      </div>
                      {s.classColors[n] && (
                        <button type="button" className="btn ghost sm" onClick={() => s.setClassColor(n, null)}>
                          <Icon name="refresh" size={12} /> Back to the default colour
                        </button>
                      )}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}

        <span className="xs faint">
          A label becomes real when you save the first box drawn under it. Until then it
          only lives in this browser, and so does any colour you pick here — a teammate
          opening the same project sees the palette default.
        </span>
      </div>

      <div className="modal-foot">
        <span className="xs muted">
          {s.classNames.length} label{s.classNames.length === 1 ? "" : "s"}
        </span>
        <button className="btn primary" onClick={onClose}>Done</button>
      </div>
    </Modal>
  );
}
