"use client";

import LearningCurve from "../components/LearningCurve";
import { VERDICT_STYLE } from "../history";
import type { Session } from "../session";
import { Empty, gradeColor, Icon, pct, READY_F1 } from "../../../lib/ui";

const median = (xs: number[]) =>
  xs.length ? [...xs].sort((a, b) => a - b)[Math.floor(xs.length / 2)] : null;

const mins = (secs: number) =>
  secs < 90 ? `${secs.toFixed(0)}s` : `${(secs / 60).toFixed(0)} min`;

/** FR-13/14/15 — the panel that answers "am I done yet?".
 *
 *  Everything here exists to stop two specific kinds of wasted afternoon:
 *  labeling a class that stopped responding, and stopping on a class that was
 *  still climbing. */
export default function InsightsPanel({ s }: { s: Session }) {
  const ready = s.advice.filter((a) => a.verdict === "ready");
  const stalled = s.advice.filter((a) => a.verdict === "plateau");
  const climbing = s.advice.filter((a) => a.verdict === "improving");

  return (
    <div className="col" style={{ gap: 14 }}>
      {/* headline recommendation */}
      <div className="card">
        <div className="card-head">
          <span className="card-title"><Icon name="spark" size={13} /> What to do next</span>
          <span className="xs muted">{s.history.length} measurement{s.history.length === 1 ? "" : "s"} recorded</span>
        </div>
        <div className="card-body col" style={{ gap: 10 }}>
          {!s.history.length ? (
            <Empty icon="target" title="No measurements yet">
              Run Evaluate at least once. Every run is recorded here, and after the second one this
              page can tell you which classes are still worth your time.
            </Empty>
          ) : (
            <>
              {stalled.length > 0 && (
                <div className="note warn">
                  <Icon name="alert" size={15} />
                  <span>
                    <strong>Stop labeling {stalled.map((a) => a.cls).join(", ")}.</strong>{" "}
                    The last rounds of examples barely moved accuracy. More of the same will not help —
                    try a lower detection threshold, tighter crops around the object, or splitting the
                    class into two that look more alike within themselves.
                  </span>
                </div>
              )}
              {ready.length > 0 && (
                <div className="note ok">
                  <Icon name="check" size={15} />
                  <span>
                    <strong>{ready.map((a) => a.cls).join(", ")} {ready.length === 1 ? "is" : "are"} ready.</strong>{" "}
                    Above the {pct(READY_F1)} bar — hand the rest of these to the model instead of drawing them.
                  </span>
                </div>
              )}
              {climbing.length > 0 && (
                <div className="note info">
                  <Icon name="info" size={15} />
                  <span>
                    <strong>Keep labeling {climbing.map((a) => a.cls).join(", ")}.</strong>{" "}
                    Still climbing — a few more examples, then evaluate again.
                  </span>
                </div>
              )}
              {!stalled.length && !ready.length && !climbing.length && (
                <span className="xs muted">Evaluate once more to get a trend.</span>
              )}
            </>
          )}
        </div>
      </div>

      {/* curve */}
      <div className="card">
        <div className="card-head">
          <span className="card-title"><Icon name="chart" size={13} /> Learning curve</span>
          {s.history.length > 0 && (
            <button className="btn ghost sm" onClick={s.resetHistory} title="Discard the recorded measurements for this output folder">
              <Icon name="trash" size={13} /> Reset
            </button>
          )}
        </div>
        <div className="card-body">
          <LearningCurve history={s.history} classes={s.classNames} color={s.color} />
        </div>
      </div>

      {/* per-class detail */}
      {s.advice.some((a) => a.verdict !== "cold") && (
        <div className="card">
          <div className="card-head">
            <span className="card-title"><Icon name="layers" size={13} /> Class by class</span>
          </div>
          <div className="card-body col" style={{ gap: 12 }}>
            {s.advice.map((a) => {
              const style = VERDICT_STYLE[a.verdict];
              return (
                <div key={a.cls} className="col" style={{ gap: 7 }}>
                  <div className="row between wrap" style={{ gap: 8 }}>
                    <span className="row" style={{ gap: 8 }}>
                      <span className="dot" style={{ color: s.color(a.cls) }} />
                      <strong style={{ fontSize: 13 }}>{a.cls}</strong>
                      <span className={`chip ${style.chip}`}>{style.label}</span>
                    </span>
                    <span className="row xs muted num" style={{ gap: 12 }}>
                      <span>{a.prompts} examples</span>
                      <span style={{ color: gradeColor(a.f1), fontWeight: 600 }}>F1 {pct(a.f1)}</span>
                      {a.delta !== null && (
                        <span style={{ color: a.delta >= 0.02 ? "var(--ok)" : "var(--faint)" }}>
                          {a.delta >= 0 ? "+" : ""}{(a.delta * 100).toFixed(1)} last round
                        </span>
                      )}
                    </span>
                  </div>
                  <div className="track" style={{ height: 5 }}>
                    <span style={{ width: `${a.f1 * 100}%`, background: gradeColor(a.f1) }} />
                  </div>
                  <span className="xs muted">{a.headline} — {a.detail}</span>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {/* Success metrics, §7 of the requirements doc. */}
      <div className="card">
        <div className="card-head">
          <span className="card-title"><Icon name="clock" size={13} /> Effort saved</span>
          <span className="xs faint">this session</span>
        </div>
        <div className="card-body col" style={{ gap: 8 }}>
          <div className="metric-grid">
            {[
              ["Time to first auto-label", s.firstAutoSecs === null ? "—" : mins(s.firstAutoSecs)],
              ["Median time per image", median(s.labelSecs) === null ? "—" : `${median(s.labelSecs)!.toFixed(0)}s`],
              ["Corrected in review", s.auto.size ? `${s.reviewed}/${s.auto.size}` : "—"],
              ["Hand-labeled share", s.images.length ? `${((s.labeled.size / s.images.length) * 100).toFixed(0)}%` : "—"],
            ].map(([k, v]) => (
              <div key={k} className="metric">
                <span className="metric-k">{k}</span>
                <span className="metric-v">{v}</span>
              </div>
            ))}
          </div>
          <span className="xs faint">
            Measured from when this session was opened and lost on reload — enough to compare against
            labeling by hand, not a permanent record.
          </span>
        </div>
      </div>
    </div>
  );
}
