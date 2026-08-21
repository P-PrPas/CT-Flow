"use client";

/** FR-13/14/15 — "do I need to label more, or is this class done?"
 *
 *  Every Evaluate run is appended to a per-project history, which is what
 *  turns a bare F1 number into a trend: the learning curve, the plateau
 *  warning, and the next-action advice all read from here.
 *
 *  Stored server-side in <input_dir>/.ctflow/_bank/eval_history.json, so the
 *  curve belongs to the dataset rather than to one person's browser.
 */

import * as api from "./api";
import type { EvalResult, PRF } from "./types";
import { READY_F1 } from "./ui";

export type EvalPoint = {
  ts: number;
  conf: number;
  /** instances in the bank per class at the moment of this evaluate */
  prompts: Record<string, number>;
  totalPrompts: number;
  overall: PRF;
  perClass: Record<string, PRF>;
};

/** The curve is a nicety -- a history read or write must never take a labeling
 *  action down with it. */
const safely = async (p: Promise<{ history: unknown[] }>): Promise<EvalPoint[]> => {
  try {
    return (await p).history as EvalPoint[];
  } catch {
    return [];
  }
};

export const loadHistory = (inputDir: string): Promise<EvalPoint[]> =>
  inputDir ? safely(api.getHistory(inputDir)) : Promise.resolve([]);

export function appendHistory(
  inputDir: string,
  result: EvalResult,
  prompts: Record<string, number>
): Promise<EvalPoint[]> {
  const point: EvalPoint = {
    ts: Date.now(),
    conf: result.conf,
    prompts,
    totalPrompts: Object.values(prompts).reduce((a, b) => a + b, 0),
    overall: result.overall,
    perClass: result.per_class,
  };
  return safely(api.addHistory(inputDir, point));
}

export const clearHistory = (inputDir: string): Promise<EvalPoint[]> =>
  safely(api.dropHistory(inputDir));

// ------------------------------------------------------------------ advice

export type Verdict = "ready" | "improving" | "plateau" | "cold";

export type ClassAdvice = {
  cls: string;
  f1: number;
  prompts: number;
  verdict: Verdict;
  /** F1 change since the previous evaluate where this class gained prompts */
  delta: number | null;
  headline: string;
  detail: string;
};

const PLATEAU_GAIN = 0.02; // F1 gain below this counts as "not moving"
const PLATEAU_RUNS = 2;    // ...for this many consecutive prompt-adding evaluates

/** Reads the history for one class and decides what the user should do next.
 *  This is the whole point of the tool telling you when to stop: labeling a
 *  class that stopped responding is the single most demoralizing way to spend
 *  an afternoon. */
export function adviseClass(cls: string, history: EvalPoint[]): ClassAdvice {
  const seen = history.filter((p) => p.perClass[cls]);
  const last = seen[seen.length - 1];
  const f1 = last?.perClass[cls]?.f1 ?? 0;
  const prompts = last?.prompts[cls] ?? 0;

  // Only compare against runs where this class actually got more examples --
  // an evaluate at a different conf with the same bank proves nothing.
  const growth: { gain: number }[] = [];
  for (let i = seen.length - 1; i > 0 && growth.length < PLATEAU_RUNS; i--) {
    const cur = seen[i];
    const prev = seen[i - 1];
    if ((cur.prompts[cls] ?? 0) > (prev.prompts[cls] ?? 0)) {
      growth.push({ gain: (cur.perClass[cls]?.f1 ?? 0) - (prev.perClass[cls]?.f1 ?? 0) });
    }
  }
  const delta = growth.length ? growth[0].gain : null;
  const plateaued = growth.length >= PLATEAU_RUNS && growth.every((g) => g.gain < PLATEAU_GAIN);

  if (seen.length === 0) {
    return {
      cls, f1, prompts, delta, verdict: "cold",
      headline: "No measurement yet",
      detail: "Run Evaluate once to get a baseline for this class.",
    };
  }
  if (f1 >= READY_F1) {
    return {
      cls, f1, prompts, delta, verdict: "ready",
      headline: "Ready for auto-label",
      detail: `F1 ${(f1 * 100).toFixed(0)}% is above the ${(READY_F1 * 100).toFixed(0)}% bar. More hand-labeling here has little left to buy.`,
    };
  }
  if (plateaued) {
    return {
      cls, f1, prompts, delta, verdict: "plateau",
      headline: "Stalled — stop labeling this one",
      detail: `The last ${PLATEAU_RUNS} rounds added examples but moved F1 by under ${(PLATEAU_GAIN * 100).toFixed(0)}%. Change the approach (lower conf, tighter crops, split the class) rather than adding more of the same.`,
    };
  }
  return {
    cls, f1, prompts, delta, verdict: "improving",
    headline: "Still improving — keep going",
    detail: delta !== null
      ? `Last round of examples moved F1 by ${delta >= 0 ? "+" : ""}${(delta * 100).toFixed(1)} points.`
      : "Label a few more, then evaluate again to see the trend.",
  };
}

export function adviseAll(classes: string[], history: EvalPoint[]): ClassAdvice[] {
  return classes.map((c) => adviseClass(c, history)).sort((a, b) => a.f1 - b.f1);
}

export const VERDICT_STYLE: Record<Verdict, { chip: string; note: string; label: string }> = {
  ready: { chip: "ok", note: "ok", label: "Ready" },
  improving: { chip: "brand", note: "info", label: "Improving" },
  plateau: { chip: "warn", note: "warn", label: "Stalled" },
  cold: { chip: "", note: "", label: "No data" },
};
