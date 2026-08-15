"use client";

import { useState } from "react";
import EvalOverlay, { EVAL_LEGEND } from "../components/EvalOverlay";
import { imgUrl } from "../lib/api";
import type { Session } from "../lib/session";
import { Empty, fileOf, gradeColor, Icon, pct, Term } from "../lib/ui";

type Filter = "all" | "errors" | "clean";

/** What the model got right and wrong, image by image. The per-image grid is
 *  the fastest way to see *why* a number is low -- consistently missing small
 *  objects reads very differently from boxes in the wrong place. */
export default function ReportPanel({ s }: { s: Session }) {
  const [filter, setFilter] = useState<Filter>("all");

  if (!s.evalResult) {
    return (
      <div className="card">
        <Empty
          icon="target"
          title="No measurement yet"
          action={<button className="btn primary" onClick={() => s.setPanel("pool")}>
            <Icon name="arrowLeft" size={14} /> Back to labeling
          </button>}
        >
          Run Evaluate from the Label tab once you have a test set with answer keys.
        </Empty>
      </div>
    );
  }

  const r = s.evalResult;
  const shown = r.per_image.filter((i) =>
    filter === "all" ? true : filter === "errors" ? i.fp + i.fn > 0 : i.fp + i.fn === 0
  );
  const errorImages = r.per_image.filter((i) => i.fp + i.fn > 0).length;

  return (
    <div className="col" style={{ gap: 14 }}>
      <div className="card">
        <div className="card-head">
          <span className="card-title"><Icon name="chart" size={13} /> Overall</span>
          <span className="xs muted">
            {r.images} test image{r.images === 1 ? "" : "s"} · {pct(r.iou)} overlap required · threshold {r.conf}
          </span>
        </div>
        <div className="card-body col" style={{ gap: 14 }}>
          <div className="metric-grid">
            <div className="metric">
              <span className="metric-k">F1</span>
              <span className="metric-v" style={{ color: gradeColor(r.overall.f1) }}>{pct(r.overall.f1)}</span>
            </div>
            <div className="metric">
              <span className="metric-k">
                <Term explain="Of the boxes the model drew, how many were right.">Precision</Term>
              </span>
              <span className="metric-v">{pct(r.overall.precision)}</span>
            </div>
            <div className="metric">
              <span className="metric-k">
                <Term explain="Of the real objects present, how many the model found.">Recall</Term>
              </span>
              <span className="metric-v">{pct(r.overall.recall)}</span>
            </div>
            <div className="metric">
              <span className="metric-k">Found</span>
              <span className="metric-v" style={{ color: "var(--ok)" }}>{r.overall.tp}</span>
            </div>
            <div className="metric">
              <span className="metric-k">False alarms</span>
              <span className="metric-v" style={{ color: "var(--warn)" }}>{r.overall.fp}</span>
            </div>
            <div className="metric">
              <span className="metric-k">Missed</span>
              <span className="metric-v" style={{ color: "var(--bad)" }}>{r.overall.fn}</span>
            </div>
          </div>

          <div className="col" style={{ gap: 8 }}>
            <span className="card-title">Per class</span>
            {Object.entries(r.per_class).map(([n, m]) => (
              <div key={n} className="col" style={{ gap: 5 }}>
                <div className="row between xs">
                  <span className="row" style={{ gap: 7 }}>
                    <span className="dot" style={{ color: s.color(n) }} />
                    <span style={{ fontWeight: 500 }}>{n}</span>
                  </span>
                  <span className="muted num">
                    F1 {pct(m.f1)} · P {pct(m.precision)} · R {pct(m.recall)} · {m.tp} found, {m.fp} false, {m.fn} missed
                  </span>
                </div>
                <div className="track" style={{ height: 5 }}>
                  <span style={{ width: `${m.f1 * 100}%`, background: gradeColor(m.f1) }} />
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>

      <div className="card">
        <div className="card-head" style={{ flexWrap: "wrap", rowGap: 8 }}>
          <div className="row wrap" style={{ gap: 14 }}>
            {EVAL_LEGEND.map((l) => (
              <span key={l.key} className="row xs muted" style={{ gap: 6 }}>
                <span
                  style={{
                    width: 12, height: 9, borderRadius: 2,
                    border: `2px ${l.key.startsWith("tp-") || l.key === "fn" ? "solid" : "dashed"} ${l.color}`,
                  }}
                />
                {l.label}
              </span>
            ))}
          </div>
          <div className="row" style={{ gap: 4 }}>
            {([["all", `All ${r.per_image.length}`], ["errors", `With errors ${errorImages}`], ["clean", `Clean ${r.per_image.length - errorImages}`]] as [Filter, string][])
              .map(([k, label]) => (
                <button key={k} className="step" aria-selected={filter === k} onClick={() => setFilter(k)}>
                  {label}
                </button>
              ))}
          </div>
        </div>
        <div className="card-body">
          {shown.length ? (
            <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(260px, 1fr))", gap: 14 }}>
              {shown.map((img) => (
                <button
                  key={img.image}
                  onClick={() => s.setZoomed(img)}
                  className="col"
                  style={{
                    gap: 6, cursor: "zoom-in", background: "none", border: 0, padding: 0,
                    textAlign: "left", color: "inherit", font: "inherit",
                  }}
                  title="Open full size"
                >
                  <EvalOverlay src={imgUrl(img.image)} gt={img.gt} pred={img.pred} />
                  <span className="mono truncate xs">{fileOf(img.image)}</span>
                  <span className="row xs" style={{ gap: 10 }}>
                    <span style={{ color: "var(--ok)" }}>{img.tp} found</span>
                    <span style={{ color: img.fp ? "var(--warn)" : "var(--faint)" }}>{img.fp} false</span>
                    <span style={{ color: img.fn ? "var(--bad)" : "var(--faint)" }}>{img.fn} missed</span>
                  </span>
                </button>
              ))}
            </div>
          ) : (
            <Empty icon="check" title="Nothing in this view">
              {filter === "errors" ? "The model made no mistakes on the test set." : "Every test image has at least one error."}
            </Empty>
          )}
        </div>
      </div>

      {s.zoomed && (
        <div className="scrim" onClick={() => s.setZoomed(null)} style={{ cursor: "zoom-out" }}>
          <div
            className="col"
            onClick={(e) => e.stopPropagation()}
            style={{ maxWidth: "min(94vw, 1400px)", maxHeight: "92vh", gap: 10, cursor: "default" }}
          >
            <div className="row between">
              <span className="mono truncate">{s.zoomed.image}</span>
              <button className="btn ghost icon" onClick={() => s.setZoomed(null)} aria-label="Close">
                <Icon name="x" size={15} />
              </button>
            </div>
            <div style={{ overflow: "auto" }}>
              <EvalOverlay src={imgUrl(s.zoomed.image)} gt={s.zoomed.gt} pred={s.zoomed.pred} />
            </div>
            <span className="row xs" style={{ gap: 14 }}>
              <span style={{ color: "var(--ok)" }}>{s.zoomed.tp} found</span>
              <span style={{ color: "var(--warn)" }}>{s.zoomed.fp} false alarms</span>
              <span style={{ color: "var(--bad)" }}>{s.zoomed.fn} missed</span>
            </span>
          </div>
        </div>
      )}
    </div>
  );
}
