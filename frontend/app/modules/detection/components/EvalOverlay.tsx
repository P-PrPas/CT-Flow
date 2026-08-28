"use client";

import { useState } from "react";

export type EvalBox = {
  cls: string;
  box: [number, number, number, number];
  matched: boolean;
  conf?: number;
};

/** Ground truth vs. prediction on one test image, color-coded by match status
 *  so a glance tells you whether an error is a miss or a false alarm. Solid
 *  outlines are truth, dashed are the model. */
export const EVAL_LEGEND = [
  { color: "var(--ok)", label: "Truth found", key: "tp-gt" },
  { color: "var(--bad)", label: "Truth missed (FN)", key: "fn" },
  { color: "var(--info)", label: "Prediction correct (TP)", key: "tp" },
  { color: "var(--warn)", label: "Prediction wrong (FP)", key: "fp" },
];

export default function EvalOverlay({ src, gt, pred }: { src: string; gt: EvalBox[]; pred: EvalBox[] }) {
  const [size, setSize] = useState({ w: 1, h: 1 });

  const rect = (b: number[]) => ({
    left: `${(b[0] / size.w) * 100}%`,
    top: `${(b[1] / size.h) * 100}%`,
    width: `${((b[2] - b[0]) / size.w) * 100}%`,
    height: `${((b[3] - b[1]) / size.h) * 100}%`,
  });

  return (
    <div className="canvas-wrap">
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img
        src={src}
        alt=""
        onLoad={(e) => setSize({ w: e.currentTarget.naturalWidth, h: e.currentTarget.naturalHeight })}
        style={{ width: "100%", display: "block" }}
      />
      {gt.map((b, i) => {
        const c = b.matched ? "var(--ok)" : "var(--bad)";
        return (
          <div key={`g${i}`} style={{ position: "absolute", ...rect(b.box), border: `2px solid ${c}`, pointerEvents: "none", borderRadius: 2 }}>
            <span className="box-label" style={{ bottom: -16, color: c, background: "rgba(4,8,16,.8)" }}>
              truth: {b.cls}{!b.matched && " · missed"}
            </span>
          </div>
        );
      })}
      {pred.map((b, i) => {
        const c = b.matched ? "var(--info)" : "var(--warn)";
        return (
          <div key={`p${i}`} style={{ position: "absolute", ...rect(b.box), border: `2px dashed ${c}`, pointerEvents: "none", borderRadius: 2 }}>
            <span className="box-label" style={{ top: -16, color: c, background: "rgba(4,8,16,.8)" }}>
              model: {b.cls}{b.conf !== undefined && ` ${b.conf.toFixed(2)}`}{!b.matched && " · wrong"}
            </span>
          </div>
        );
      })}
    </div>
  );
}
