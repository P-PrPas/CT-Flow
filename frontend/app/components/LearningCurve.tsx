"use client";

import { useMemo, useRef, useState } from "react";
import type { EvalPoint } from "../lib/history";
import { Empty, Icon, pct } from "../lib/ui";

/** FR-13 — F1 against the number of examples taught, one line per class.
 *
 *  Class colors are fixed by the backend (config.LABEL_COLORS) on purpose: a
 *  class must be the same color on the canvas, in the bank list, and on this
 *  curve. Validated against the dark surface for CVD separation and 3:1
 *  contrast; the palette sits slightly brighter than the ideal band, which is
 *  the right trade for keeping one color per class everywhere.
 *
 *  Identity is never color-alone -- there is a legend, end-of-line direct
 *  labels up to 4 series, and a table view. */

const W = 760, H = 250;
const PAD = { t: 16, r: 74, b: 34, l: 40 };
const PLOT = { w: W - PAD.l - PAD.r, h: H - PAD.t - PAD.b };

export default function LearningCurve({
  history, classes, color,
}: {
  history: EvalPoint[];
  classes: string[];
  color: (c: string) => string;
}) {
  const svgRef = useRef<SVGSVGElement>(null);
  const [hover, setHover] = useState<number | null>(null);
  const [table, setTable] = useState(false);

  const points = useMemo(
    () => [...history].sort((a, b) => a.totalPrompts - b.totalPrompts),
    [history]
  );

  const xMax = Math.max(1, ...points.map((p) => p.totalPrompts));
  const x = (v: number) => PAD.l + (v / xMax) * PLOT.w;
  const y = (f1: number) => PAD.t + (1 - Math.min(1, Math.max(0, f1))) * PLOT.h;

  const series = useMemo(
    () =>
      classes
        .map((cls) => ({
          cls,
          color: color(cls),
          pts: points
            .filter((p) => p.perClass[cls])
            .map((p) => ({ x: p.totalPrompts, y: p.perClass[cls].f1 })),
        }))
        .filter((s) => s.pts.length > 0),
    [classes, points, color]
  );

  if (points.length < 2) {
    return (
      <Empty icon="chart" title="Not enough measurements yet">
        The curve needs at least two Evaluate runs. Label a few more images, run Evaluate
        again, and this shows whether the extra work actually moved accuracy — the fastest
        way to tell &ldquo;keep going&rdquo; from &ldquo;this class is done&rdquo;.
      </Empty>
    );
  }

  const onMove = (e: React.PointerEvent) => {
    const r = svgRef.current!.getBoundingClientRect();
    const vx = ((e.clientX - r.left) / r.width) * W;
    let best = 0, bestD = Infinity;
    points.forEach((p, i) => {
      const d = Math.abs(x(p.totalPrompts) - vx);
      if (d < bestD) { bestD = d; best = i; }
    });
    setHover(best);
  };

  const hp = hover !== null ? points[hover] : null;
  const ticks = [0, 0.25, 0.5, 0.75, 1];

  return (
    <div className="col" style={{ gap: 10 }}>
      <div className="row between">
        <span className="xs muted">
          Accuracy (F1) against the number of examples you have taught
        </span>
        <button className="btn ghost sm" onClick={() => setTable((t) => !t)}>
          <Icon name={table ? "chart" : "layers"} size={13} />
          {table ? "Show chart" : "Show numbers"}
        </button>
      </div>

      {table ? (
        <div style={{ overflowX: "auto" }}>
          <table className="xs" style={{ width: "100%", borderCollapse: "collapse" }}>
            <thead>
              <tr style={{ color: "var(--faint)", textAlign: "left" }}>
                <th style={th}>Examples</th>
                <th style={th}>Overall F1</th>
                {series.map((s) => <th key={s.cls} style={th}>{s.cls}</th>)}
              </tr>
            </thead>
            <tbody>
              {points.map((p, i) => (
                <tr key={i} style={{ borderTop: "1px solid var(--line-soft)" }}>
                  <td style={{ ...td, fontFamily: "var(--mono)" }}>{p.totalPrompts}</td>
                  <td style={{ ...td, fontFamily: "var(--mono)" }}>{pct(p.overall.f1)}</td>
                  {series.map((s) => (
                    <td key={s.cls} style={{ ...td, fontFamily: "var(--mono)" }}>
                      {p.perClass[s.cls] ? pct(p.perClass[s.cls].f1) : "—"}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <div style={{ position: "relative" }}>
          <svg
            ref={svgRef}
            viewBox={`0 0 ${W} ${H}`}
            width="100%"
            style={{ display: "block", overflow: "visible" }}
            onPointerMove={onMove}
            onPointerLeave={() => setHover(null)}
            role="img"
            aria-label="Learning curve: F1 per class against examples taught"
          >
            {/* recessive grid */}
            {ticks.map((t) => (
              <g key={t}>
                <line x1={PAD.l} x2={PAD.l + PLOT.w} y1={y(t)} y2={y(t)} stroke="var(--line-soft)" strokeWidth="1" />
                <text x={PAD.l - 8} y={y(t) + 3.5} textAnchor="end" fontSize="10" fill="var(--faint)" fontFamily="var(--mono)">
                  {t.toFixed(2)}
                </text>
              </g>
            ))}
            {/* the "good enough to auto-label" line */}
            <line
              x1={PAD.l} x2={PAD.l + PLOT.w} y1={y(0.75)} y2={y(0.75)}
              stroke="var(--ok)" strokeWidth="1" strokeDasharray="4 4" opacity=".55"
            />
            <text x={PAD.l + PLOT.w + 6} y={y(0.75) + 3.5} fontSize="9.5" fill="var(--ok)">
              ready bar
            </text>

            <line x1={PAD.l} x2={PAD.l + PLOT.w} y1={PAD.t + PLOT.h} y2={PAD.t + PLOT.h} stroke="var(--line)" strokeWidth="1" />
            <text x={PAD.l} y={H - 8} fontSize="10" fill="var(--faint)" fontFamily="var(--mono)">0</text>
            <text x={PAD.l + PLOT.w} y={H - 8} textAnchor="end" fontSize="10" fill="var(--faint)" fontFamily="var(--mono)">
              {xMax} examples
            </text>

            {hp && (
              <line
                x1={x(hp.totalPrompts)} x2={x(hp.totalPrompts)} y1={PAD.t} y2={PAD.t + PLOT.h}
                stroke="var(--brand)" strokeWidth="1" opacity=".5"
              />
            )}

            {series.map((s) => {
              const d = s.pts.map((p, i) => `${i ? "L" : "M"}${x(p.x)},${y(p.y)}`).join(" ");
              const last = s.pts[s.pts.length - 1];
              return (
                <g key={s.cls}>
                  <path d={d} fill="none" stroke={s.color} strokeWidth="2" strokeLinejoin="round" strokeLinecap="round" />
                  {s.pts.map((p, i) => (
                    <circle
                      key={i} cx={x(p.x)} cy={y(p.y)} r="3.2"
                      fill={s.color} stroke="var(--surface)" strokeWidth="2"
                    />
                  ))}
                  {/* direct labels: identity without reading the legend */}
                  {series.length <= 4 && (
                    <text x={x(last.x) + 8} y={y(last.y) + 3.5} fontSize="10.5" fill={s.color} fontWeight="600">
                      {s.cls}
                    </text>
                  )}
                </g>
              );
            })}
          </svg>

          {hp && (
            <div
              style={{
                position: "absolute", top: 0,
                left: `${(x(hp.totalPrompts) / W) * 100}%`,
                transform: `translateX(${x(hp.totalPrompts) > W / 2 ? "-104%" : "4%"})`,
                pointerEvents: "none", zIndex: 5,
                background: "#0a1020", border: "1px solid var(--line)",
                borderRadius: "var(--r)", padding: "8px 10px", minWidth: 150,
                boxShadow: "var(--shadow-3)",
              }}
            >
              <div className="xs faint num" style={{ marginBottom: 4 }}>
                {hp.totalPrompts} examples · conf {hp.conf}
              </div>
              <div className="row between xs" style={{ marginBottom: 3 }}>
                <span className="muted">Overall</span>
                <span className="num" style={{ fontWeight: 600 }}>{pct(hp.overall.f1)}</span>
              </div>
              {series.map((s) => (
                <div key={s.cls} className="row between xs" style={{ gap: 12 }}>
                  <span className="row" style={{ gap: 5 }}>
                    <span className="dot" style={{ color: s.color }} />
                    <span className="muted">{s.cls}</span>
                  </span>
                  <span className="num">{hp.perClass[s.cls] ? pct(hp.perClass[s.cls].f1) : "—"}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      <div className="row wrap" style={{ gap: 12 }}>
        {series.map((s) => (
          <span key={s.cls} className="row xs muted" style={{ gap: 6 }}>
            <span className="dot" style={{ color: s.color }} />
            {s.cls}
          </span>
        ))}
      </div>
    </div>
  );
}

const th: React.CSSProperties = { padding: "6px 10px", fontWeight: 500, whiteSpace: "nowrap" };
const td: React.CSSProperties = { padding: "6px 10px", whiteSpace: "nowrap" };
