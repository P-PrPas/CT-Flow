import type { EvalBox } from "./components/EvalOverlay";

export type BankSummary = {
  classes: { name: string; count: number }[];
  labeled: string[];
  auto: string[];
  // null until the first box is saved -- the model picker stays open until then.
  model: string | null;
};

/** One selectable YOLOE checkpoint, as reported by GET /api/config.
 *  `available` is whether the weight is already on disk on the server --
 *  picking one that isn't means the first predict/label call pays an
 *  auto-download tax, or fails outright with no local internet path. */
export type ModelInfo = { id: string; family: string; size: string; note: string; available: boolean };

/** `sig` is an 8x8 grayscale thumbnail of the image (FR-18): cheap enough to
 *  ship with every score, good enough to spot near-duplicate frames. */
export type Score = { conf: number; cls: string | null; sig?: number[] };

export type PRF = { precision: number; recall: number; f1: number; tp: number; fp: number; fn: number };

export type EvalImage = { image: string; gt: EvalBox[]; pred: EvalBox[]; tp: number; fp: number; fn: number };

export type EvalResult = {
  overall: PRF;
  per_class: Record<string, PRF>;
  per_image: EvalImage[];
  images: number;
  iou: number;
  conf: number;
};
