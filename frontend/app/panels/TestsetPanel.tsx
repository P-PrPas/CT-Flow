"use client";

import { useState } from "react";
import BoxCanvas from "../components/BoxCanvas";
import { imgUrl } from "../lib/api";
import type { Session } from "../lib/session";
import { Empty, fileOf, Icon, stemOf, Term } from "../lib/ui";

/** Ground-truth prep. A separate flow on purpose: these labels are written
 *  straight to disk and the images are never turned into teaching examples --
 *  a test set the model has seen cannot measure anything. */
export default function TestsetPanel({ s }: { s: Session }) {
  const done = s.tsLabeled.length;
  const total = s.tsImages.length;

  return (
    <div className="workspace">
      <div className="col" style={{ gap: 14 }}>
        <div className="note info">
          <Icon name="info" size={15} />
          <span>
            These images are the <strong>answer key</strong>. Label them carefully by hand — the model
            never learns from them, it is only ever graded against them. 10–20 varied images is enough
            to be useful.
          </span>
        </div>

        <div className="card">
          <div className="card-head" style={{ flexWrap: "wrap", rowGap: 8 }}>
            <div className="row wrap" style={{ gap: 6 }}>
              {s.tsClasses.map((n, i) => (
                <button
                  key={n}
                  className="swatch"
                  aria-pressed={s.tsCls === n}
                  onClick={() => s.setTsCls(n)}
                  style={{ color: s.tsColor(n) }}
                >
                  <span className="dot" />
                  <span style={{ color: "var(--text)" }}>{n}</span>
                  {i < 9 && <span className="key">{i + 1}</span>}
                </button>
              ))}
              <input
                value={s.tsCls}
                onChange={(e) => s.setTsCls(e.target.value)}
                placeholder="new class…"
                aria-label="Class for the next box"
                style={{ width: 128, height: 28, minHeight: 28, fontSize: 12.5 }}
              />
            </div>
            <div className="row" style={{ gap: 6 }}>
              <button className="btn ghost icon sm" onClick={s.ts.undo} disabled={!s.ts.canUndo} title="Undo (Ctrl+Z)">
                <Icon name="undo" size={14} />
              </button>
              <button className="btn ghost icon sm" onClick={s.ts.redo} disabled={!s.ts.canRedo} title="Redo (Ctrl+Shift+Z)">
                <Icon name="redo" size={14} />
              </button>
              <button className="btn ghost icon sm" onClick={() => s.ts.set([])} disabled={!s.ts.boxes.length} title="Clear (Esc)">
                <Icon name="trash" size={14} />
              </button>
            </div>
          </div>

          <div className="card-body col" style={{ gap: 12 }}>
            {s.tsCurrent ? (
              <>
                <div className="row between wrap" style={{ gap: 8 }}>
                  <span className="mono muted truncate" title={s.tsCurrent}>{fileOf(s.tsCurrent)}</span>
                  <div className="row" style={{ gap: 6 }}>
                    {s.tsLabeledSet.has(stemOf(s.tsCurrent)) && (
                      <span className="chip ok"><Icon name="check" size={12} /> answer key written</span>
                    )}
                    <span className="chip">{s.ts.boxes.length} box{s.ts.boxes.length === 1 ? "" : "es"}</span>
                  </div>
                </div>

                <BoxCanvas
                  src={imgUrl(s.tsCurrent)}
                  boxes={s.ts.boxes}
                  savedBoxes={s.tsSavedBoxes}
                  color={s.tsColor}
                  selected={s.selected}
                  onSelect={s.setSelected}
                  onAdd={(b) => s.ts.set((cur) => [...cur, { cls: s.tsCls || "item", box: b }])}
                  onRemove={(i) => s.ts.set((cur) => cur.filter((_, idx) => idx !== i))}
                  onUpdate={(i, b) => s.ts.set((cur) => cur.map((x, idx) => (idx === i ? { ...x, box: b } : x)))}
                />

                <div className="row wrap between" style={{ gap: 10 }}>
                  <span className="xs faint">
                    Mark every real object — a missed one counts against the model. Drag a box to move it,
                    grab a corner to resize.
                  </span>
                  <div className="row wrap" style={{ gap: 8 }}>
                    <button
                      className="btn ghost"
                      onClick={() => s.goToTsImage(s.tsImages.filter((p) => p !== s.tsCurrent)[0] ?? null)}
                      disabled={!s.tsImages.length}
                    >
                      <Icon name="skip" size={13} /> Skip
                    </button>
                    <label className="check" title="Add to what is already saved for this image">
                      <input
                        type="checkbox"
                        checked={s.tsUpdateMode}
                        onChange={(e) => s.setTsUpdateMode(e.target.checked)}
                      />
                      Add to existing
                    </label>
                    <button className="btn primary" onClick={s.saveTestset} disabled={!s.ts.boxes.length || s.busy}>
                      <Icon name="save" size={14} /> Save answer key
                    </button>
                  </div>
                </div>
              </>
            ) : (
              <Empty
                icon="target"
                title={total ? "All test images are labeled" : "No test images yet"}
                action={
                  !s.inputDir ? (
                    <button className="btn primary" onClick={() => s.setShowSetup(true)}>
                      <Icon name="folder" size={14} /> Open a session first
                    </button>
                  ) : !total && s.poolCandidates.length ? (
                    /* The sidebar can do this too, but an empty screen should
                       carry the one action that gets you off it. */
                    <button className="btn primary" onClick={s.addRandomFromPool} disabled={s.busy}>
                      <Icon name="spark" size={14} /> Take {Math.min(s.sampleN, s.poolCandidates.length)} at random from the pool
                    </button>
                  ) : undefined
                }
              >
                {total
                  ? "Head back to the label pool and measure accuracy."
                  : "Pull some images across from the pool on the right, then draw the correct boxes on each."}
              </Empty>
            )}
          </div>
        </div>
      </div>

      {/* ------------------------------------------------ sidebar */}
      <div className="col" style={{ gap: 14 }}>
        <div className="card">
          <div className="card-head">
            <span className="card-title"><Icon name="flag" size={13} /> Answer key progress</span>
            <span className="xs muted num">{done}/{total || 0}</span>
          </div>
          <div className="card-body tight col" style={{ gap: 8 }}>
            <div className="track">
              <span className="fill-ok" style={{ width: `${total ? (done / total) * 100 : 0}%` }} />
            </div>
            <span className="xs muted">
              {total === 0
                ? "Import some images to get started."
                : done < 10
                  ? `${10 - done} more would make the measurement a lot more trustworthy.`
                  : "Enough images for a solid measurement."}
            </span>
          </div>
        </div>

        <div className="card">
          <div className="card-head">
            <span className="card-title"><Icon name="copy" size={13} /> Bring in from the pool</span>
          </div>
          <div className="card-body tight col" style={{ gap: 10 }}>
            {!s.inputDir ? (
              <span className="xs muted">Open a session in Session setup first.</span>
            ) : !s.images.length ? (
              <span className="xs muted">Open an image folder on the Label tab — this list mirrors that pool.</span>
            ) : (
              <>
                <div className="row" style={{ gap: 8 }}>
                  <input
                    type="number" min={1} max={Math.max(1, s.poolCandidates.length)}
                    value={s.sampleN}
                    onChange={(e) => s.setSampleN(Number(e.target.value))}
                    aria-label="How many random images to import"
                    style={{ width: 68 }}
                  />
                  <button className="btn grow" onClick={s.addRandomFromPool} disabled={!s.poolCandidates.length || s.busy}>
                    <Icon name="spark" size={13} /> Add at random
                  </button>
                </div>
                <button
                  className="btn block"
                  onClick={() => s.importToTestset([...s.poolPick])}
                  disabled={!s.poolPick.size || s.busy}
                >
                  <Icon name="plus" size={13} /> Add {s.poolPick.size} selected
                </button>
                <div style={{ maxHeight: "30vh", overflow: "auto", margin: "0 -4px" }}>
                  {s.poolCandidates.map((p) => (
                    <label key={p} className="thumb-row" style={{ cursor: "pointer" }}>
                      <input
                        type="checkbox"
                        checked={s.poolPick.has(p)}
                        onChange={(e) => {
                          const next = new Set(s.poolPick);
                          if (e.target.checked) next.add(p); else next.delete(p);
                          s.setPoolPick(next);
                        }}
                      />
                      {/* eslint-disable-next-line @next/next/no-img-element */}
                      <img className="thumb" style={{ width: 40, height: 30 }} src={imgUrl(p)} alt="" loading="lazy" />
                      <span className="mono truncate">{fileOf(p)}</span>
                    </label>
                  ))}
                  {!s.poolCandidates.length && (
                    <div className="xs muted" style={{ padding: 8 }}>Every pool image is already in the test set.</div>
                  )}
                </div>
              </>
            )}
          </div>
        </div>

        <div className="card flush">
          <div className="card-head">
            <span className="card-title"><Icon name="image" size={13} /> Test images ({total})</span>
            <span className="xs faint">{s.tsPick.size} selected</span>
          </div>
          <div className="card-body tight row" style={{ gap: 8 }}>
            <button
              className="btn sm grow"
              onClick={() => s.setTsPick(s.tsPick.size === total ? new Set() : new Set(s.tsImages))}
              disabled={!total}
            >
              {s.tsPick.size === total && total ? "Select none" : "Select all"}
            </button>
            <button
              className="btn sm danger grow"
              onClick={() => s.removeFromTestset([...s.tsPick])}
              disabled={!s.tsPick.size || s.busy}
            >
              <Icon name="trash" size={13} /> Remove {s.tsPick.size}
            </button>
          </div>
          <div style={{ maxHeight: "34vh", overflow: "auto", padding: "0 8px 8px" }}>
            {s.tsImages.map((p) => (
              <div key={p} className="thumb-row" aria-current={p === s.tsCurrent} onClick={() => s.goToTsImage(p)}>
                <input
                  type="checkbox"
                  checked={s.tsPick.has(p)}
                  onClick={(e) => e.stopPropagation()}
                  onChange={(e) => {
                    const next = new Set(s.tsPick);
                    if (e.target.checked) next.add(p); else next.delete(p);
                    s.setTsPick(next);
                  }}
                />
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img className="thumb" src={imgUrl(p)} alt="" loading="lazy" />
                <div className="grow" style={{ overflow: "hidden" }}>
                  <div className="mono truncate">{fileOf(p)}</div>
                  <div className="xs" style={{ color: s.tsLabeledSet.has(stemOf(p)) ? "var(--ok)" : "var(--faint)" }}>
                    {s.tsLabeledSet.has(stemOf(p)) ? "answer key written" : "not labeled"}
                  </div>
                </div>
              </div>
            ))}
            {!total && <div className="xs muted" style={{ padding: 10 }}>Nothing imported yet.</div>}
          </div>
        </div>

        <div className="card pad">
          <span className="xs muted">
            <Term explain="Precision, recall and F1 are all measured by comparing the model's boxes against these hand-drawn ones at 50% overlap.">
              How the score is computed
            </Term>
          </span>
        </div>
      </div>
    </div>
  );
}
