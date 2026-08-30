/** The API calls that are not about labeling: signing in, browsing the server's
 *  folders, and projects.
 *
 *  Everything tied to the labeling loop lives in the module that owns it
 *  (app/modules/detection/api.ts) and reuses `request`/`post` from here. That is
 *  the boundary in one sentence: this file must not import anything from
 *  app/modules/, so a second module cannot make this one grow. */

export async function request(url: string, init?: RequestInit) {
  const res = await fetch(url, init);
  const data = await res.json();
  if (res.status === 401 && typeof window !== "undefined" && !window.location.pathname.startsWith("/entry/")) {
    window.location.assign("/entry/login");
  }
  if (!res.ok) throw new Error(data.detail ?? "request failed");
  return data;
}

export const post = (url: string, body: unknown) =>
  request(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });

export const imgUrl = (path: string) => `/api/image?path=${encodeURIComponent(path)}`;

/** A downscaled JPEG for grid views. `w` is the target width in CSS pixels;
 *  the server never upscales and clamps at 400. Cached hard by the browser
 *  (immutable), keyed on the file's mtime, so a re-scroll costs nothing. */
export const thumbUrl = (path: string, w = 200) =>
  `/api/thumb?path=${encodeURIComponent(path)}&w=${w}`;

export type AuthState = {
  /** Always true since T-27 -- signing in is mandatory, and the server will not
   *  start without a way to do it. Kept because it is what the endpoint sends. */
  enabled: boolean;
  /** The display name -- what to print. Not an identity: under OIDC a provider
   *  can change it, and two people can share one. */
  user: string | null;
  /** The caller's own attribution key: the same value as `Project.owner.oid`
   *  and the `created_by` behind `contributors`. Compare on this to answer "is
   *  this mine" -- `user` is a label, this is the identity behind it. */
  oid: string | null;
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

export function browse(path: string) {
  return request(`/api/browse?path=${encodeURIComponent(path)}`);
}

// --- projects (FR-43, FR-44, FR-50) --------------------------------------
// Generic on purpose: nothing here knows what a prompt bank is. A project has a
// folder and a task type, and which module renders it is decided by the route.

export type Person = { oid: string; username: string };

/** Someone who has actually labeled here, counted from annotations.created_by
 *  -- what happened, not who was invited, which is why there is no member list
 *  to maintain. `username` falls back to the raw subject when the provider
 *  never wrote a users row (a local login). */
export type Contributor = Person & { boxes: number };

export type Project = {
  id: number;
  input_dir: string;
  name: string;
  task_type: string;
  /** null when nobody has claimed it. */
  owner: Person | null;
  labeled: number;
  auto: number;
  contributors: Contributor[];
  created_at: string;
  updated_at: string;
};

export function listProjects(): Promise<{ projects: Project[] }> {
  return request("/api/projects");
}

export function createProject(
  name: string, input_dir: string, task_type = "detection"
): Promise<{ project: Project }> {
  return post("/api/projects", { name, input_dir, task_type });
}

/** What /p/{id} calls on mount to turn the URL into the input_dir every other
 *  endpoint speaks. */
export function getProject(id: number): Promise<{ project: Project }> {
  return request(`/api/projects/${id}`);
}

/** Rename, and/or claim an unowned project. There is no field for handing it to
 *  someone else: claiming can only fill an empty owner, never replace one. */
export function updateProject(
  id: number, patch: { name?: string; claim_ownership?: boolean }
): Promise<{ project: Project }> {
  return request(`/api/projects/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(patch),
  });
}

/** Drops the labels, never the files. `kept_on_disk` is the folder that stays,
 *  so the confirmation can say what survives. */
export function deleteProject(id: number): Promise<{ deleted: number; kept_on_disk: string }> {
  return request(`/api/projects/${id}`, { method: "DELETE" });
}
