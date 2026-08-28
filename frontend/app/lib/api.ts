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
  if (res.status === 401 && typeof window !== "undefined" && !window.location.pathname.startsWith("/entry/")) {
    window.location.assign("/entry/login");
  }
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

export type AuthState = {
  /** Always true since T-27 -- signing in is mandatory, and the server will not
   *  start without a way to do it. Kept because it is what the endpoint sends. */
  enabled: boolean;
  user: string | null;
  /** "local" is the CI and development credential path; people use "oidc". */
  mode: "local" | "oidc";
  /** Only on the logout response, and only under OIDC: where to send the
   *  browser so the provider session ends too. Clearing our own cookie alone
   *  leaves the next "sign in" silent. */
  logoutUrl?: string;
};

export function getAuth(): Promise<AuthState> {
  return request("/api/auth/me");
}

export function loginRedirect(): Promise<{ redirectUrl: string }> {
  return request("/api/public/login/redirect");
}

export function loginCallback(code: string, state: string): Promise<AuthState> {
  return post("/api/public/login/callback", { code, state });
}

export function localLogin(username: string, password: string): Promise<AuthState> {
  return post("/api/auth/login", { username, password });
}

export function logout(): Promise<AuthState> {
  return post("/api/auth/logout", {});
}

export function getConfig(): Promise<{
  roots: string[]; colors: string[]; models: ModelInfo[]; default_model: string;
}> {
  return request("/api/config");
}

export function browse(path: string) {
  return request(`/api/browse?path=${encodeURIComponent(path)}`);
}

export function getBoxes(
  input_dir: string, image: string, kind: "pool" | "test" = "pool"
): Promise<{ boxes: Box[] }> {
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
