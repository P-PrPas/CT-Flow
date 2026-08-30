"use client";

import { useState } from "react";

export type EvalBox = {
  cls: string;
  box: [number, number, number, number];
  matched: boolean;
  conf?: number;
};

/** Ground truth vs. prediction on one test image, one box per object:
 *
 *   - green solid, no label  -- a correct detection (TP). Shown so the image
 *     never looks like the model did nothing, but unlabelled so it stays quiet.
 *   - red solid, labelled    -- a real object the model missed (FN).
 *   - amber dashed, labelled -- a box the model drew that is not there (FP).
 *
 * The errors carry the labels because the errors are the reason to open the
 * grid: "why is the score low" is answered by the red and amber, not the green.
 */
export const EVAL_LEGEND = [
  { color: "var(--ok)", label: "Correct", dashed: false },
  { color: "var(--bad)", label: "Missed", dashed: false },
  { color: "var(--warn)", label: "False alarm", dashed: true },
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

      {gt.filter((b) => b.matched).map((b, i) => (
        <div
          key={`c${i}`}
          style={{
            position: "absolute", ...rect(b.box),
            border: "2px solid var(--ok)", opacity: 0.8,
            pointerEvents: "none", borderRadius: 2,
          }}
        />
      ))}

      {gt.filter((b) => !b.matched).map((b, i) => (
        <div
          key={`m${i}`}
          style={{
            position: "absolute", ...rect(b.box),
            border: "2px solid var(--bad)", pointerEvents: "none", borderRadius: 2,
          }}
        >
          <span className="box-label" style={{ bottom: -16, color: "var(--bad)", background: "rgba(4,8,16,.8)" }}>
            missed: {b.cls}
          </span>
        </div>
      ))}

      {pred.filter((b) => !b.matched).map((b, i) => (
        <div
          key={`f${i}`}
          style={{
            position: "absolute", ...rect(b.box),
            border: "2px dashed var(--warn)", pointerEvents: "none", borderRadius: 2,
          }}
        >
          <span className="box-label" style={{ top: -16, color: "var(--warn)", background: "rgba(4,8,16,.8)" }}>
            {b.cls}{b.conf !== undefined ? ` ${b.conf.toFixed(2)}` : ""}
          </span>
        </div>
      ))}
    </div>
  );
}
