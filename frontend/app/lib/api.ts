/** Thin client for backend/routers/*.py -- every fetch call in the app goes
 *  through here, so page.tsx only deals with state, not HTTP. All requests
 *  go through /api/*, proxied to FastAPI by app/api/[...path]/route.ts. */
import type { Box } from "../components/BoxCanvas";
import type { JobProgress } from "../components/ProgressBar";
import type { BankSummary, EvalResult, ModelInfo, Score } from "./types";

export type { JobProgress };

async function request(url: string, init?: RequestInit) {
  const res = await fetch(url, init);
  const data = await res.json();
  if (!res.ok) throw new Error(data.detail ?? "request failed");
  return data;
}

const post = (url: string, body: unknown) =>
  request(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });

export const imgUrl = (path: string) => `/api/image?path=${encodeURIComponent(path)}`;

export function getConfig(): Promise<{
  mode: string; roots: string[]; colors: string[]; models: ModelInfo[]; default_model: string;
}> {
  return request("/api/config");
}

export function browse(path: string) {
  return request(`/api/browse?path=${encodeURIComponent(path)}`);
}

export function getBoxes(dir: string, image: string): Promise<{ boxes: Box[] }> {
  return request(`/api/boxes?dir=${encodeURIComponent(dir)}&image=${encodeURIComponent(image)}`);
}

// --- pool -------------------------------------------------------------

export function openSession(
  input_dir: string,
  output_dir: string
): Promise<{ images: string[]; bank: BankSummary }> {
  return post("/api/session", { input_dir, output_dir });
}

export function saveLabel(
  output_dir: string, image: string, boxes: Box[], mode: "replace" | "update", model_id: string
): Promise<{ bank: BankSummary }> {
  return post("/api/label", { output_dir, image, boxes, mode, model_id });
}

/** Rewrites the image's label file directly -- no embedding extraction, for
 *  fixing an auto-generated label without treating the edit as a new prompt. */
export function relabel(
  output_dir: string, image: string, boxes: Box[], mode: "replace" | "update" = "replace"
): Promise<{ bank: BankSummary }> {
  return post("/api/relabel", { output_dir, image, boxes, mode });
}

/** FR-19 — the model's guesses for one image, drawn as drafts the user accepts
 *  or ignores. Returns [] instantly when the bank is empty. */
export function predict(
  output_dir: string, image: string, conf: number
): Promise<{ boxes: (Box & { conf: number })[] }> {
  return post("/api/predict", { output_dir, image, conf });
}

// --- evaluate history (T-07) -------------------------------------------

export function getHistory(output_dir: string): Promise<{ history: unknown[] }> {
  return request(`/api/history?output_dir=${encodeURIComponent(output_dir)}`);
}

export function addHistory(output_dir: string, point: unknown): Promise<{ history: unknown[] }> {
  return post("/api/history", { output_dir, point });
}

export function dropHistory(output_dir: string): Promise<{ history: unknown[] }> {
  return request(`/api/history?output_dir=${encodeURIComponent(output_dir)}`, { method: "DELETE" });
}

// --- test set -----------------------------------------------------------

export function openTestset(
  test_dir: string
): Promise<{ images: string[]; labeled: string[]; classes: string[] }> {
  return post("/api/testset/session", { test_dir });
}

export function importTestset(test_dir: string, images: string[]) {
  return post("/api/testset/import", { test_dir, images }) as Promise<{
    images: string[]; labeled: string[]; classes: string[]; imported: string[];
  }>;
}

export function removeTestset(test_dir: string, images: string[]) {
  return post("/api/testset/remove", { test_dir, images }) as Promise<{
    images: string[]; labeled: string[]; classes: string[]; removed: string[];
  }>;
}

export function labelTestset(
  test_dir: string, image: string, boxes: Box[], mode: "replace" | "update"
): Promise<{ classes: string[]; labeled: string[] }> {
  return post("/api/testset/label", { test_dir, image, boxes, mode });
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

export function rescorePool(output_dir: string, images: string[], onProgress: (p: JobProgress) => void) {
  return runJob("/api/score", { output_dir, images }, onProgress) as Promise<{
    scores: Record<string, Score>;
  }>;
}

export function evaluateTestSet(
  output_dir: string, test_dir: string, conf: number, onProgress: (p: JobProgress) => void
): Promise<EvalResult> {
  return runJob("/api/evaluate", { output_dir, test_dir, conf }, onProgress);
}

export function autoLabelRemaining(
  output_dir: string, images: string[], conf: number, onProgress: (p: JobProgress) => void
) {
  return runJob("/api/autolabel", { output_dir, images, conf }, onProgress) as Promise<{
    written: number; no_detection: number; no_detection_images: string[]; bank: BankSummary;
  }>;
}

/** The only sanctioned way to change a project's model after its first label
 *  -- re-extracts every taught instance's embedding under the new checkpoint.
 *  Label files are untouched; only the prompt bank's vectors change. */
export function reembedBank(
  output_dir: string, model_id: string, onProgress: (p: JobProgress) => void
) {
  return runJob("/api/reembed", { output_dir, model_id }, onProgress) as Promise<{ bank: BankSummary }>;
}
