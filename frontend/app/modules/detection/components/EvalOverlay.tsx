"use client";

import { useState } from "react";

export type EvalBox = {
  cls: string;
  box: [number, number, number, number];
  matched: boolean;
  conf?: number;
};

/** Ground truth vs. prediction on one test image. Only the errors are drawn:
 *  a red solid box for something the model missed, an amber dashed box for
 *  something it drew that is not there. Correct detections are the majority on
 *  any usable model and add nothing to "why is the score low" -- they show only
 *  in the zoomed view, as a faint hairline with no label. */
export const EVAL_LEGEND = [
  { color: "var(--bad)", label: "Missed", key: "fn" },
  { color: "var(--warn)", label: "False alarm", key: "fp" },
];

export default function EvalOverlay({
  src, gt, pred, showCorrect = false,
}: {
  src: string;
  gt: EvalBox[];
  pred: EvalBox[];
  /** Draw the correct detections too, as an unlabelled hairline. Off for the
   *  grid thumbnails, on when the image is opened full size. */
  showCorrect?: boolean;
}) {
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

      {showCorrect &&
        gt.filter((b) => b.matched).map((b, i) => (
          <div
            key={`c${i}`}
            style={{
              position: "absolute", ...rect(b.box),
              border: "1px solid var(--line)", opacity: 0.55,
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
