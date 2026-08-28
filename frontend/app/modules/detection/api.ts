/** The labeling loop, module-scoped: every endpoint here is meaningful only for
 *  object detection against a YOLOE prompt bank. The shared plumbing --
 *  `request`, `post`, auth, browsing, projects -- comes from app/lib/api. */
import { imgUrl, post, request } from "../../lib/api";
import type { Box } from "./components/BoxCanvas";
import type { JobProgress } from "../../components/ProgressBar";
import type { BankSummary, EvalResult, ModelInfo, Score } from "./types";

export type { JobProgress };

/** Re-exported, not redefined: GET /api/image is shared, but every caller of it
 *  is in this module, so one import per panel beats two. */
export { imgUrl };

/** Carries the box colours and the checkpoint catalog, so it belongs to the
 *  module that renders both rather than to the shared client. */
export function getConfig(): Promise<{
  roots: string[]; colors: string[]; models: ModelInfo[]; default_model: string;
}> {
  return request("/api/config");
}
export function getBoxes(
  input_dir: string, image: string, kind: "pool" | "test" = "pool"
): Promise<{ boxes: Box[]; labeled_by: { oid: string; username: string }[] }> {
  return request(
    `/api/boxes?input_dir=${encodeURIComponent(input_dir)}&image=${encodeURIComponent(image)}&kind=${kind}`
  );
}

// --- pool -------------------------------------------------------------

/** One folder in, everything else comes back in one shot: the prompt bank
 *  (labels + taught examples) and the test-set manifest both live in a fixed
 *  subfolder of input_dir server-side (see backend/deps.py), so there is
 *  nothing else to pick and no second request to make. */
export function openSession(input_dir: string): Promise<{
  images: string[]; bank: BankSummary;
  testset: { images: string[]; labeled: string[]; classes: string[] };
  /** FR-51: the bank holds taught classes while the database holds no images
   *  for this project -- a half-wipe, and the one state where what the model
   *  knows and what the app knows have come apart. */
  bank_orphaned: boolean;
}> {
  return post("/api/session", { input_dir });
}

export function saveLabel(
  input_dir: string, image: string, boxes: Box[], mode: "replace" | "update", model_id: string
): Promise<{ bank: BankSummary }> {
  return post("/api/label", { input_dir, image, boxes, mode, model_id });
}

/** Rewrites the image's label file directly -- no embedding extraction, for
 *  fixing an auto-generated label without treating the edit as a new prompt. */
export function relabel(
  input_dir: string, image: string, boxes: Box[], mode: "replace" | "update" = "replace"
): Promise<{ bank: BankSummary }> {
  return post("/api/relabel", { input_dir, image, boxes, mode });
}

/** FR-19 — the model's guesses for one image, drawn as drafts the user accepts
 *  or ignores. Returns [] instantly when the bank is empty. */
export function predict(
  input_dir: string, image: string, conf: number
): Promise<{ boxes: (Box & { conf: number })[] }> {
  return post("/api/predict", { input_dir, image, conf });
}

// --- evaluate history (T-07) -------------------------------------------

export function getHistory(input_dir: string): Promise<{ history: unknown[] }> {
  return request(`/api/history?input_dir=${encodeURIComponent(input_dir)}`);
}

export function addHistory(input_dir: string, point: unknown): Promise<{ history: unknown[] }> {
  return post("/api/history", { input_dir, point });
}

export function dropHistory(input_dir: string): Promise<{ history: unknown[] }> {
  return request(`/api/history?input_dir=${encodeURIComponent(input_dir)}`, { method: "DELETE" });
}

// --- test set -------------------------------------------------------------
// Flags pool images as held out -- no copy, no second folder. See
// backend/services/groundtruth.py's manifest.

export function importTestset(input_dir: string, images: string[]) {
  return post("/api/testset/import", { input_dir, images }) as Promise<{
    images: string[]; labeled: string[]; classes: string[]; imported: string[];
  }>;
}

export function removeTestset(input_dir: string, images: string[]) {
  return post("/api/testset/remove", { input_dir, images }) as Promise<{
    images: string[]; labeled: string[]; classes: string[]; removed: string[];
  }>;
}

export function labelTestset(
  input_dir: string, image: string, boxes: Box[], mode: "replace" | "update"
): Promise<{ classes: string[]; labeled: string[] }> {
  return post("/api/testset/label", { input_dir, image, boxes, mode });
}

// --- background jobs: evaluate / autolabel / rescore --------------------

function jobStatus(jobId: string) {
  return request(`/api/jobs/${jobId}`);
}

/** Starts a job and polls /api/jobs/{id} until it finishes, reporting
 *  progress along the way -- the shared engine behind rescorePool,
 *  evaluateTestSet, and autoLabelRemaining below. */
export function runJob(url: string, body: unknown, onProgress: (p: JobProgress) => void): Promise<any> {
  return post(url, body).then(
    (started) =>
      new Promise((resolve, reject) => {
        const poll = () => {
          jobStatus(started.job_id)
            .then((j) => {
              onProgress({ done: j.done, total: j.total, startedAt: j.started_at, now: j.now });
              if (j.finished) {
                if (j.error) reject(new Error(j.error));
                else resolve(j.result);
              } else {
                setTimeout(poll, 400);
              }
            })
            .catch(reject);
        };
        onProgress({ done: 0, total: started.total, startedAt: 0, now: 0 });
        poll();
      })
  );
}

export function rescorePool(input_dir: string, images: string[], onProgress: (p: JobProgress) => void) {
  return runJob("/api/score", { input_dir, images }, onProgress) as Promise<{
    scores: Record<string, Score>;
  }>;
}

export function evaluateTestSet(
  input_dir: string, conf: number, onProgress: (p: JobProgress) => void
): Promise<EvalResult> {
  return runJob("/api/evaluate", { input_dir, conf }, onProgress);
}

export function autoLabelRemaining(
  input_dir: string, images: string[], conf: number, onProgress: (p: JobProgress) => void
) {
  return runJob("/api/autolabel", { input_dir, images, conf }, onProgress) as Promise<{
    written: number; no_detection: number; no_detection_images: string[]; bank: BankSummary;
  }>;
}

/** The only sanctioned way to change a project's model after its first label
 *  -- re-extracts every taught instance's embedding under the new checkpoint.
 *  Label files are untouched; only the prompt bank's vectors change. */
export function reembedBank(
  input_dir: string, model_id: string, onProgress: (p: JobProgress) => void
) {
  return runJob("/api/reembed", { input_dir, model_id }, onProgress) as Promise<{ bank: BankSummary }>;
}

// --- working alongside someone else (FR-48, FR-49) ------------------------

export type LiveState = {
  labeled: string[];
  auto: string[];
  testset_labeled: string[];
  /** image path -> the name of whoever is on it. Names, never subjects. */
  claims: Record<string, string>;
};

/** What changes while you are not the one changing it. Polled while the
 *  workspace is open, so it is deliberately small: no bank summary, because
 *  classes and the locked model only change when *you* label and your own save
 *  already returns them. */
export function getState(input_dir: string): Promise<LiveState> {
  return request(`/api/state?input_dir=${encodeURIComponent(input_dir)}`);
}

/** "I am working on this image", so the other person's queue points elsewhere.
 *  Advice, not a lock -- a save is never refused because of it. Re-claiming
 *  your own image renews it, which is what makes calling this on a timer a
 *  heartbeat rather than a special case. */
export function claimImage(input_dir: string, image: string, release = false) {
  return post("/api/claim", { input_dir, image, release }) as Promise<{
    claims: Record<string, string>;
  }>;
}
