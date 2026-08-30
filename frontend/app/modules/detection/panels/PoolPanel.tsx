"use client";

import { useState } from "react";
import BoxCanvas from "../components/BoxCanvas";
import Confirm from "../../../components/Confirm";
import ModelPicker from "../components/ModelPicker";
import { imgUrl } from "../api";
import { VERDICT_STYLE } from "../history";
import type { Session } from "../session";
import { Empty, fileOf, gradeColor, HelpDot, Icon, pct, READY_F1, Term } from "../../../lib/ui";

/** How many rows the Queue shows. It answers "what do I label next", which
 *  needs the head of the least-confident order, not the whole folder -- browsing
 *  everything is the Gallery tab's job (docs/GALLERY_PLAN.md T-39). Keyboard
 *  next/prev still walks the full s.sortedPool. */
const QUEUE_CAP = 60;

export default function PoolPanel({ s }: { s: Session }) {
  const [confirmAuto, setConfirmAuto] = useState(false);

  const f1 = s.evalResult?.overall.f1 ?? null;
  const notReady = f1 !== null && f1 < READY_F1;

  const prog = s.progressBuckets;
  const doneCount = prog.hand + prog.model + prog.test;
  const total = prog.total || 1;

  const askAutoLabel = () => {
    if (notReady) setConfirmAuto(true);
    else s.runAuto();
  };

  return (
    <div className="workspace">
      {/* ------------------------------------------------ canvas side */}
      <div className="col" style={{ gap: 14 }}>
        {/* FR-50 -- on a shared project, "who drew this" is the difference
            between trusting a box and re-checking it. Names come from the
            server already resolved; the raw subject never reaches the UI. */}
        {s.labeledBy.length > 0 && (
          <span className="xs muted row" style={{ gap: 5 }}>
            <Icon name="user" size={12} />
            labeled by {s.labeledBy.join(", ")}
          </span>
        )}

        {s.isReview && (
          /* FR-25 — the single most misunderstood thing in the tool: an edit
             here does not, by itself, change what the model learned. Say so,
             and put the alternative right next to it. */
          <div className="note warn">
            <Icon name="alert" size={15} />
            <span>
              {s.current && s.auto.has(s.current) ? (
                <>
                  <strong>The model labeled this one.</strong> Fix the boxes, then <em>Save</em> to
                  keep the correction — that alone does <em>not</em> teach the model. Use{" "}
                  <em>Save &amp; teach</em> when this image should become an example too.
                </>
              ) : (
                <>
                  <strong>Already labeled by hand.</strong> Edits <em>Save</em> as a plain correction —
                  the model does not relearn from it. Use <em>Save &amp; teach</em> only if the fixed
                  boxes should also update its examples.
                </>
              )}
            </span>
          </div>
        )}

        <div className="card">
          <div className="card-head" style={{ flexWrap: "wrap", rowGap: 8 }}>
            <div className="row wrap" style={{ gap: 6 }}>
              {/* FR-21 — the class sticks between boxes and images; keys 1-9 switch it. */}
              {s.classNames.map((n, i) => (
                <button
                  key={n}
                  className="swatch"
                  aria-pressed={s.cls === n}
                  onClick={() => s.setCls(n)}
                  style={{ color: s.color(n) }}
                  title={`Draw ${n} boxes (key ${i + 1})`}
                >
                  <span className="dot" />
                  <span style={{ color: "var(--text)" }}>{n}</span>
                  {i < 9 && <span className="key">{i + 1}</span>}
                </button>
              ))}
              {!s.isReview && (
                <input
                  value={s.cls}
                  onChange={(e) => s.setCls(e.target.value)}
                  placeholder="new class…"
                  aria-label="Class for the next box"
                  style={{ width: 128, height: 28, minHeight: 28, fontSize: 12.5 }}
                />
              )}
            </div>

            <div className="row" style={{ gap: 6 }}>
              <button className="btn ghost icon sm" onClick={s.pool.undo} disabled={!s.pool.canUndo} title="Undo (Ctrl+Z)">
                <Icon name="undo" size={14} />
              </button>
              <button className="btn ghost icon sm" onClick={s.pool.redo} disabled={!s.pool.canRedo} title="Redo (Ctrl+Shift+Z)">
                <Icon name="redo" size={14} />
              </button>
              <button
                className="btn ghost icon sm"
                onClick={() => s.pool.set([])}
                disabled={!s.pool.boxes.length}
                title="Clear all boxes (Esc)"
              >
                <Icon name="trash" size={14} />
              </button>
              {/* FR-22 — conveyor frames barely move between shots. */}
              <button
                className="btn sm"
                onClick={() => s.clipboard && s.pool.set(s.clipboard.boxes)}
                disabled={!s.clipboard}
                title={s.clipboard ? `Copy ${s.clipboard.boxes.length} box(es) from ${fileOf(s.clipboard.from)} (C)` : "Save an image first"}
              >
                <Icon name="copy" size={13} /> Copy last
              </button>
            </div>
          </div>

          <div className="card-body col" style={{ gap: 12 }}>
            {s.current ? (
              <>
                <div className="row between wrap" style={{ gap: 8 }}>
                  <span className="mono muted truncate" title={s.current}>{fileOf(s.current)}</span>
                  <div className="row" style={{ gap: 6 }}>
                    {s.auto.has(s.current) ? (
                      <span className="chip warn"><Icon name="bot" size={12} /> Machine-labeled</span>
                    ) : s.labeled.has(s.current) ? (
                      <span className="chip ok"><Icon name="check" size={12} /> Labeled by hand</span>
                    ) : s.scores[s.current] ? (
                      <span className="chip"><Icon name="gauge" size={12} /> confidence {s.scores[s.current].conf.toFixed(2)}</span>
                    ) : null}
                    <span className="chip">{s.pool.boxes.length} box{s.pool.boxes.length === 1 ? "" : "es"}</span>
                  </div>
                </div>

                {/* FR-19 — the model already looked at this image; say what it
                    found and let one click turn it into work-in-progress. */}
                {(s.drafting || s.drafts.length > 0) && (
                  <div className="note info">
                    <Icon name="wand" size={15} />
                    {s.drafting ? (
                      <span>Looking at this image…</span>
                    ) : (
                      <>
                        <span className="grow">
                          <strong>The model suggests {s.drafts.length} box{s.drafts.length === 1 ? "" : "es"}</strong>
                          {" "}— dotted outlines. Click one to select it and hit × to drop a bad guess.
                          Nothing is saved until you take the rest.
                        </span>
                        <div className="row" style={{ gap: 6 }}>
                          <button className="btn sm" onClick={() => s.setDrafts([])}>Ignore</button>
                          <button className="btn primary sm" onClick={s.acceptDrafts}>
                            <Icon name="check" size={13} /> Use them (A)
                          </button>
                        </div>
                      </>
                    )}
                  </div>
                )}

                <BoxCanvas
                  src={imgUrl(s.current)}
                  label={fileOf(s.current)}
                  boxes={s.pool.boxes}
                  draftBoxes={s.drafts}
                  onRemoveDraft={(i) => s.setDrafts((cur) => cur.filter((_, idx) => idx !== i))}
                  color={s.color}
                  selected={s.selected}
                  onSelect={s.setSelected}
                  onAdd={(b) => {
                    const cls = s.isReview
                      ? (s.classNames.includes(s.cls) ? s.cls : s.classNames[0] ?? "item")
                      : (s.cls || "item");
                    s.pool.set((cur) => [...cur, { cls, box: b }]);
                  }}
                  onRemove={(i) => s.pool.set((cur) => cur.filter((_, idx) => idx !== i))}
                  onUpdate={(i, b) => s.pool.set((cur) => cur.map((x, idx) => (idx === i ? { ...x, box: b } : x)))}
                />

                <div className="row wrap between" style={{ gap: 10 }}>
                  <span className="xs faint">
                    Drag to draw (works inside another box too) · click a box&rsquo;s edge to select it, then drag to move or grab a corner to resize
                  </span>
                  <div className="row wrap" style={{ gap: 8 }}>
                    <button
                      className="btn ghost"
                      onClick={() =>
                        s.goToImage(
                          // Mid review pass (this image is machine-labeled): keep
                          // walking the machine-labeled ones. Otherwise: next thing
                          // that still needs a first label.
                          s.current && s.auto.has(s.current)
                            ? s.images.find((p) => p !== s.current && s.auto.has(p))
                              ?? s.nextTodo(s.doneSet, s.current)
                            : s.nextTodo(s.doneSet, s.current)
                        )
                      }
                      disabled={!s.images.length}
                    >
                      <Icon name="skip" size={13} /> Skip
                    </button>

                    <label className="check" title="Add these boxes to what is already saved for this image, instead of replacing it">
                      <input
                        type="checkbox"
                        checked={s.updateMode}
                        onChange={(e) => s.setUpdateMode(e.target.checked)}
                      />
                      Add to existing
                    </label>

                    {s.isReview ? (
                      <>
                        <button className="btn" onClick={s.saveReview} disabled={s.busy}
                          title="Save these boxes as the label. Does not change what the model learned.">
                          <Icon name="save" size={14} /> Save
                        </button>
                        <button
                          className="btn primary"
                          onClick={s.teachFromReview}
                          disabled={!s.pool.boxes.length || s.busy}
                          title="Save the boxes AND add this image to the model's examples"
                        >
                          <Icon name="brain" size={14} /> Save &amp; teach
                        </button>
                      </>
                    ) : (
                      <button
                        className="btn primary"
                        onClick={s.saveLabel}
                        disabled={!s.pool.boxes.length || s.busy}
                        title="Save and jump to the next image (Enter)"
                      >
                        <Icon name="brain" size={14} />
                        {s.simple ? "Teach the model" : s.updateMode ? "Update examples" : "Save example"}
                      </button>
                    )}
                  </div>
                </div>

                <label className="check" title="Runs the model on each image as you open it (only once something has been taught)">
                  <input
                    type="checkbox"
                    checked={s.preAnnotate}
                    onChange={(e) => s.setPreAnnotate(e.target.checked)}
                  />
                  <Icon name="wand" size={13} />
                  Pre-fill each image with the model&rsquo;s guesses, so you correct instead of draw
                </label>
              </>
            ) : s.images.length ? (
              <Empty icon="check" title="Nothing left in the queue">
                Every image in this folder is labeled. Measure accuracy on the test set, or start a
                project on another folder.
              </Empty>
            ) : (
              /* The route opens the session itself, so an empty pool is the
                 folder loading -- or a folder whose images went away since the
                 project was created. */
              <Empty
                icon="image"
                title={s.busy ? "Opening…" : "No images in this folder"}
                action={!s.busy && <a className="btn" href="/">
                  <Icon name="folder" size={14} /> All projects
                </a>}
              >
                {s.busy
                  ? "Reading the folder and the taught examples."
                  : "The folder this project points at has no images in it any more."}
              </Empty>
            )}
          </div>
        </div>
      </div>

      {/* ------------------------------------------------ sidebar */}
      <div className="col" style={{ gap: 14 }}>
        {/* FR-23 — progress is permanently on screen, not buried in a status line. */}
        <div className="card">
          <div className="card-head">
            <h2 className="card-title"><Icon name="flag" size={13} /> Progress</h2>
            <span className="xs muted num">{doneCount}/{s.images.length || 0}</span>
          </div>
          <div className="card-body col tight" style={{ gap: 10 }}>
            <div className="track">
              <span className="fill-ok" style={{ width: `${(prog.hand / total) * 100}%` }} />
              <span className="fill-brand" style={{ width: `${(prog.model / total) * 100}%` }} />
              <span className="fill-info" style={{ width: `${(prog.test / total) * 100}%` }} />
              <span className="fill-warn" style={{ width: `${(prog.nothing / total) * 100}%` }} />
            </div>
            <div className="row wrap xs" style={{ gap: 10 }}>
              <span className="row" style={{ gap: 5 }}>
                <span className="dot" style={{ color: "var(--ok)" }} /> {prog.hand} by hand
              </span>
              <span className="row" style={{ gap: 5 }}>
                <span className="dot" style={{ color: "var(--brand)" }} /> {prog.model} by model
              </span>
              {prog.test > 0 && (
                <span className="row" style={{ gap: 5 }}>
                  <span className="dot" style={{ color: "var(--info)" }} /> {prog.test} test set
                </span>
              )}
              {prog.nothing > 0 && (
                <span className="row" style={{ gap: 5 }}>
                  <span className="dot" style={{ color: "var(--warn)" }} /> {prog.nothing} found nothing
                </span>
              )}
              {prog.left > 0 && (
                <span className="row muted" style={{ gap: 5 }}>
                  <span className="dot" style={{ color: "var(--line)" }} /> {prog.left} left
                </span>
              )}
            </div>
          </div>
        </div>

        {/* FR-37/T-05 follow-up — the model picker used to live only in Setup,
            which collapses out of view once a session opens. It's reachable
            here too now, not just before "Open session" -- still read-only
            once the bank has locked a model (see ModelPicker). */}
        <div className="card">
          <div className="card-head">
            <h2 className="card-title"><Icon name="brain" size={13} /> Model</h2>
            <HelpDot text="Bigger sizes are slower but generally more accurate. Fixed the moment the first box is saved into an output folder — start a new one to try a different model. The dot shows whether the weight is already on the server (green) or has to be fetched on first use (red)." />
          </div>
          <div className="card-body">
            <ModelPicker s={s} label={false} />
          </div>
        </div>

        {/* FR-28 — "12 with nothing found" is a number; these are the images. */}
        {s.noDetection.length > 0 && (
          <div className="note warn">
            <Icon name="alert" size={15} />
            <span className="grow">
              <strong>{s.noDetection.length} image{s.noDetection.length === 1 ? "" : "s"} got no label.</strong>{" "}
              The model found nothing above {s.conf.toFixed(2)} there — either they are genuinely
              empty, or they hold something it has never been taught.
              <button
                className="btn ghost sm"
                style={{ marginTop: 8 }}
                onClick={() => s.goToImage(s.noDetection[0])}
              >
                Open the first one <Icon name="arrowRight" size={13} />
              </button>
            </span>
          </div>
        )}

        {/* prompt bank + per-class verdict */}
        <div className="card">
          <div className="card-head">
            <h2 className="card-title">
              <Icon name="brain" size={13} />
              <Term explain="Every box you save is turned into a visual example the model compares new images against. More varied examples, better matching.">
                {s.simple ? "Taught examples" : "Prompt bank"}
              </Term>
            </h2>
            <span className="xs muted num">{s.bankTotal}</span>
          </div>
          <div className="card-body col tight" style={{ gap: 8 }}>
            {s.bank?.classes.length ? (
              s.bank.classes.map((c) => {
                const a = s.advice.find((x) => x.cls === c.name);
                const style = a ? VERDICT_STYLE[a.verdict] : null;
                return (
                  <div key={c.name} className="row between" style={{ gap: 8 }}>
                    <span className="row truncate" style={{ gap: 7 }}>
                      <span className="dot" style={{ color: s.color(c.name) }} />
                      <span className="truncate">{c.name}</span>
                    </span>
                    <span className="row" style={{ gap: 6 }}>
                      {a && a.verdict !== "cold" && (
                        <span className={`chip ${style!.chip}`} title={a.detail}>{style!.label}</span>
                      )}
                      <span className="muted num xs">{c.count}</span>
                    </span>
                  </div>
                );
              })
            ) : (
              <span className="xs muted">Nothing taught yet — draw a box and save it.</span>
            )}
          </div>
        </div>

        {/* readiness */}
        <div className="card">
          <div className="card-head">
            <h2 className="card-title"><Icon name="gauge" size={13} /> Readiness</h2>
            <HelpDot text="Both of these run the model over every image, so they only run when you ask." />
          </div>
          <div className="card-body col tight" style={{ gap: 12 }}>
            <div className="col" style={{ gap: 6 }}>
              <div className="row between xs">
                <span className="muted row" style={{ gap: 5 }}>
                  <Term explain="How sure the model must be before it draws a box. Lower finds more, including more mistakes; higher is stricter.">
                    Detection threshold
                  </Term>
                </span>
                <span className="num">{s.conf.toFixed(2)}</span>
              </div>
              <input
                type="range" min={0.01} max={0.95} step={0.01}
                value={s.conf}
                onChange={(e) => s.setConf(Number(e.target.value))}
                aria-label="Detection confidence threshold"
              />
            </div>

            <div className="row" style={{ gap: 8 }}>
              <button
                className="btn grow"
                onClick={s.runEval}
                disabled={!s.tsLabeled.length || !s.bank?.classes.length || s.busy}
                title={s.tsLabeled.length ? "" : "Label at least one test image first"}
              >
                <Icon name="target" size={14} /> {s.simple ? "Check accuracy" : "Evaluate"}
              </button>
              <button
                className="btn grow"
                onClick={s.rescore}
                disabled={!s.bank?.classes.length || s.busy}
                title="Re-runs the model over the unlabeled images to re-order the queue"
              >
                <Icon name="refresh" size={14} /> Re-check pool
                {s.staleScores > 0 && <span className="chip warn">+{s.staleScores}</span>}
              </button>
            </div>
            {s.staleScores >= 5 && (
              <span className="xs faint">
                {s.staleScores} examples taught since the last re-check — the queue order is out of date.
              </span>
            )}

            {s.evalResult && (
              <>
                <div className="metric-grid">
                  <div className="metric">
                    <span className="metric-k">F1</span>
                    <span className="metric-v" style={{ color: gradeColor(s.evalResult.overall.f1) }}>
                      {pct(s.evalResult.overall.f1)}
                    </span>
                  </div>
                  <div className="metric">
                    <span className="metric-k">Precision</span>
                    <span className="metric-v">{pct(s.evalResult.overall.precision)}</span>
                  </div>
                  <div className="metric">
                    <span className="metric-k">Recall</span>
                    <span className="metric-v">{pct(s.evalResult.overall.recall)}</span>
                  </div>
                </div>
                <button className="btn ghost sm" onClick={() => s.setPanel("report")}>
                  See what it got wrong <Icon name="arrowRight" size={13} />
                </button>
              </>
            )}

            <button
              className="btn primary block"
              onClick={askAutoLabel}
              disabled={!s.evalResult || !s.remaining.length || s.busy}
              title={s.evalResult ? "" : "Measure accuracy on the test set first"}
            >
              <Icon name="bot" size={14} /> Label the remaining {s.remaining.length}
            </button>
            {!s.evalResult && (
              <span className="xs faint">
                Locked until you have measured accuracy — handing the rest to an unmeasured model
                is how a bad dataset gets made.
              </span>
            )}
          </div>
        </div>

        {/* queue */}
        <div className="card flush">
          <div className="card-head">
            <h2 className="card-title"><Icon name="layers" size={13} /> Queue</h2>
            <span className="xs faint">least confident first</span>
          </div>
          <div className="card-body tight" style={{ paddingTop: 8, paddingBottom: 8 }}>
            {/* FR-18 — needs signatures from a Re-check to have anything to
                compare, so it says so instead of silently doing nothing. */}
            <label className="check" title="Avoids handing you five near-identical frames in a row">
              <input
                type="checkbox"
                checked={s.spread}
                onChange={(e) => s.setSpread(e.target.checked)}
              />
              Also spread picks across different-looking images
            </label>
            {s.spread && !Object.keys(s.scores).length && (
              <span className="xs faint">Re-check the pool once to give this something to compare.</span>
            )}
          </div>
          <div style={{ maxHeight: "46vh", overflow: "auto", padding: "0 8px 8px" }}>
            {s.sortedPool.slice(0, QUEUE_CAP).map((p) => (
              <button
                type="button"
                key={p}
                className="thumb-row"
                aria-current={p === s.current ? "true" : undefined}
                onClick={() => s.goToImage(p)}
              >
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img className="thumb" src={imgUrl(p)} alt="" loading="lazy" />
                <div className="grow" style={{ overflow: "hidden" }}>
                  <div className="mono truncate">{fileOf(p)}</div>
                  <div className="xs muted">
                    {s.labeled.has(p) ? "labeled by hand"
                      : s.auto.has(p) ? "labeled by model"
                        : s.scores[p] ? `${s.scores[p].conf.toFixed(2)} ${s.scores[p].cls ?? ""}`
                          : "not checked yet"}
                  </div>
                  {/* FR-49 -- the queue already skips these when picking what
                      is next; saying so is what stops it looking like a bug. */}
                  {s.heldByOthers.has(p) && (
                    <div className="xs warn row" style={{ gap: 4 }}>
                      <Icon name="user" size={11} /> {s.claims[p]} is on this one
                    </div>
                  )}
                </div>
              </button>
            ))}
            {!s.images.length && (
              <div className="xs muted" style={{ padding: 10 }}>
                {s.busy ? "Opening…" : "No images in this folder."}
              </div>
            )}
            {s.sortedPool.length > QUEUE_CAP && (
              <button
                className="btn ghost sm"
                style={{ width: "100%", marginTop: 6 }}
                onClick={() => s.setPanel("gallery")}
              >
                <Icon name="layers" size={13} /> See all {s.images.length} in the gallery
              </button>
            )}
          </div>
        </div>
      </div>

      {/* FR-27 */}
      {confirmAuto && s.evalResult && (
        <Confirm
          title="Accuracy is below the bar"
          confirmLabel={`Label them anyway`}
          onConfirm={s.runAuto}
          onClose={() => setConfirmAuto(false)}
          body={
            <>
              <p style={{ margin: 0, fontSize: 13, lineHeight: 1.6 }}>
                The model scores <strong style={{ color: gradeColor(s.evalResult.overall.f1) }}>
                  F1 {pct(s.evalResult.overall.f1)}</strong> on your test set, under the{" "}
                {pct(READY_F1)} bar. Labeling {s.remaining.length} images now means someone corrects
                a lot of them later — usually slower than teaching a few more examples first.
              </p>
              {s.advice.filter((a) => a.verdict !== "ready" && a.verdict !== "cold").map((a) => (
                <div key={a.cls} className={`note ${VERDICT_STYLE[a.verdict].note}`}>
                  <Icon name={a.verdict === "plateau" ? "alert" : "info"} size={14} />
                  <span><strong>{a.cls}</strong> — {a.headline}. {a.detail}</span>
                </div>
              ))}
            </>
          }
        />
      )}
    </div>
  );
}
