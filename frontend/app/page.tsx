"use client";

/** Home: what work exists on this server, who owns it, and how far along it is.
 *
 *  Deliberately knows nothing about YOLOE. It renders projects by `task_type`
 *  and links to the route that owns that type -- so a second labeling module is
 *  a sibling folder under app/modules/ and one more branch in `workspaceHref`,
 *  not a change to this page. Nothing here may import from app/modules/. */

import { useCallback, useEffect, useState } from "react";
import type { FormEvent, ReactNode } from "react";
import { useRouter } from "next/navigation";
import Confirm from "./components/Confirm";
import DirPicker from "./components/DirPicker";
import * as api from "./lib/api";
import { BrandMark, Empty, fileOf, Icon, Soon, Tip } from "./lib/ui";

/** Where a project of this type is labeled. One entry today; a second module
 *  adds a branch here and a folder, and every card above keeps working. */
const workspaceHref = (p: api.Project) => `/p/${p.id}`;

export default function Home() {
  const router = useRouter();
  const [auth, setAuth] = useState<api.AuthState | null>(null);
  const [projects, setProjects] = useState<api.Project[] | null>(null);
  const [error, setError] = useState("");
  const [creating, setCreating] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState<api.Project | null>(null);

  const reload = useCallback(
    () =>
      api.listProjects()
        .then((d) => { setProjects(d.projects); setError(""); })
        .catch((e: Error) => setError(e.message)),
    []
  );

  useEffect(() => {
    api.getAuth()
      .then(setAuth)
      // A failed call means the API is unreachable, not that nobody is signed
      // in -- but the login screen is where both are recoverable from.
      .catch(() => setAuth({ enabled: true, user: null, oid: null, mode: "oidc" }));
  }, []);

  useEffect(() => {
    if (!auth) return;
    if (!auth.user) { router.replace("/entry/login"); return; }
    reload();
  }, [auth, router, reload]);

  if (!auth || !auth.user) {
    return <main className="row" style={{ minHeight: "100dvh", justifyContent: "center" }}>Loading…</main>;
  }
  const me = auth.user;

  // Split on the subject, never on the display name. Under OIDC `auth.user` is
  // whatever the provider calls someone today while `owner.oid` is their `sub`
  // -- comparing the two matches nothing, and every project would land under
  // "everyone else's" on exactly the deployment that matters. With local
  // accounts the two strings happen to be equal, which is how that goes
  // unnoticed all the way through development.
  const mine = projects?.filter((p) => p.owner?.oid === auth.oid) ?? [];
  const others = projects?.filter((p) => p.owner?.oid !== auth.oid) ?? [];

  return (
    <>
      <header className="appbar">
        <div className="appbar-inner">
          <div className="row" style={{ gap: 10 }}>
            <span className="brand-mark"><BrandMark /></span>
            <span className="col" style={{ gap: 1 }}>
              <span className="brand-name">CT-Flow</span>
              <span className="brand-sub">Connected Tech</span>
            </span>
          </div>
          <span className="spacer" />
          <button
            className="chip btn-like"
            title="Sign out"
            onClick={() => api.logout()
              .then((out) => window.location.assign(out.logoutUrl || "/entry/login"))
              .catch(() => window.location.assign("/entry/login"))}
          >
            <Icon name="user" size={12} /> {me}
          </button>
        </div>
      </header>

      <main className="col" style={{ gap: 18, padding: "18px var(--s5) var(--s6)", maxWidth: 1200, margin: "0 auto" }}>
        <div className="row between wrap" style={{ gap: 12 }}>
          <div className="col" style={{ gap: 2 }}>
            <h1 style={{ margin: 0, fontSize: 19 }}>Projects</h1>
            <span className="xs muted">One folder of images is one project.</span>
          </div>
          <button className="btn primary" onClick={() => setCreating(true)}>
            <Icon name="plus" size={14} /> New project
          </button>
        </div>

        {error && (
          <div className="note bad" role="alert">
            <Icon name="alert" size={15} />
            <span>{error}</span>
          </div>
        )}

        {projects === null && !error && <span className="muted">Loading…</span>}

        {projects?.length === 0 && (
          <Empty
            icon="folder"
            title="No projects yet"
            action={
              <button className="btn primary" onClick={() => setCreating(true)}>
                <Icon name="plus" size={14} /> New project
              </button>
            }
          >
            Point one at a folder of images on the server and start labeling.
          </Empty>
        )}

        {mine.length > 0 && (
          <Section title="Yours">
            {mine.map((p) => (
              <ProjectCard key={p.id} p={p} meOID={auth.oid} onChanged={reload}
                onDelete={() => setConfirmDelete(p)} onError={setError} />
            ))}
          </Section>
        )}
        {others.length > 0 && (
          <Section
            title="Everyone else's"
            hint="Open any of them — this is a shared server, not a set of private folders."
          >
            {others.map((p) => (
              <ProjectCard key={p.id} p={p} meOID={auth.oid} onChanged={reload}
                onDelete={() => setConfirmDelete(p)} onError={setError} />
            ))}
          </Section>
        )}
      </main>

      {creating && (
        <CreateDialog onClose={() => setCreating(false)} onCreated={(p) => router.push(workspaceHref(p))} />
      )}

      {confirmDelete && (
        <Confirm
          title={`Delete “${confirmDelete.name}”?`}
          tone="bad"
          icon="trash"
          confirmLabel="Delete project"
          onClose={() => setConfirmDelete(null)}
          onConfirm={() =>
            api.deleteProject(confirmDelete.id).then(reload).catch((e: Error) => setError(e.message))
          }
          body={
            /* Says what survives, without recommending the one thing that
               breaks: a new project on this folder starts its class list from
               scratch while the prompt bank still holds the old order, which is
               the divergence FR-51 exists to catch and nothing warns about yet
               (docs/PHASE2_WORKSPACE.md #8). */
            <>
              Removes its labels, classes and test set from the database. The images in{" "}
              <code>{confirmDelete.input_dir}</code> and the taught examples in its{" "}
              <code>.ctflow</code> folder are <strong>not</strong> touched — nothing on
              disk is deleted. Labeling this folder again from a new project means
              teaching it again from scratch.
            </>
          }
        />
      )}
    </>
  );
}

function Section({ title, hint, children }: { title: string; hint?: string; children: ReactNode }) {
  return (
    <section className="col" style={{ gap: 10 }}>
      <div className="row" style={{ gap: 8, alignItems: "baseline" }}>
        <h2 style={{ margin: 0, fontSize: 14 }}>{title}</h2>
        {hint && <span className="xs faint">{hint}</span>}
      </div>
      <div className="project-grid">{children}</div>
    </section>
  );
}

function ProjectCard({
  p, meOID, onChanged, onDelete, onError,
}: {
  p: api.Project; meOID: string | null;
  onChanged: () => void; onDelete: () => void; onError: (m: string) => void;
}) {
  const router = useRouter();
  const [renaming, setRenaming] = useState(false);
  const [name, setName] = useState(p.name);

  const rename = (e: FormEvent) => {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed || trimmed === p.name) { setRenaming(false); return; }
    api.updateProject(p.id, { name: trimmed })
      .then(() => { setRenaming(false); onChanged(); })
      .catch((err: Error) => { setRenaming(false); onError(err.message); });
  };

  return (
    <div className="card project-card">
      <div className="card-body col" style={{ gap: 10 }}>
        <div className="row between" style={{ gap: 8, alignItems: "flex-start" }}>
          {renaming ? (
            <form onSubmit={rename} className="row grow" style={{ gap: 6 }}>
              <input className="grow" autoFocus value={name} aria-label="Project name"
                onChange={(e) => setName(e.target.value)} onBlur={rename} />
            </form>
          ) : (
            <button className="link-title" onClick={() => router.push(workspaceHref(p))}>{p.name}</button>
          )}
          <span className="chip">{p.task_type}</span>
        </div>

        <span className="xs faint mono truncate" title={p.input_dir}>{fileOf(p.input_dir)}</span>

        <div className="row wrap xs muted" style={{ gap: 10 }}>
          <span className="row" style={{ gap: 4 }}>
            <Icon name="user" size={12} /> {p.owner ? p.owner.username : "no owner"}
          </span>
          <span className="row" style={{ gap: 4 }}>
            <Icon name="clock" size={12} /> {ago(p.updated_at)}
          </span>
        </div>

        {/* Counts are what the database holds, not what the folder holds: the
            total number of images needs a directory listing, and doing one per
            card on every page load is not what a summary is for. */}
        <div className="row wrap xs" style={{ gap: 8 }}>
          <Tip text="Images labeled by a person.">
            <span className="chip ok"><Icon name="check" size={12} /> {p.labeled} labeled</span>
          </Tip>
          <Tip text="Images the model labeled once its readiness was good enough.">
            <span className="chip"><Icon name="bot" size={12} /> {p.auto} auto</span>
          </Tip>
          {p.labeled + p.auto === 0 && <span className="faint">nothing labeled yet</span>}
        </div>

        {p.contributors.length > 0 && (
          <Tip text="Derived from who actually saved each box — not a membership list, so it says what happened rather than who was invited.">
            <span className="xs muted row wrap" style={{ gap: 6 }}>
              <Icon name="layers" size={12} />
              {p.contributors.map((c) => `${c.username} (${c.boxes})`).join(" · ")}
            </span>
          </Tip>
        )}

        <div className="row wrap" style={{ gap: 6, marginTop: 2 }}>
          <button className="btn sm primary" onClick={() => router.push(workspaceHref(p))}>
            <Icon name="play" size={13} /> Open
          </button>
          <button className="btn sm ghost" onClick={() => { setName(p.name); setRenaming(true); }}>
            Rename
          </button>
          {!p.owner && (
            <Tip text="Nobody owns this yet. Claiming can only fill an empty owner — it never takes one.">
              <button
                className="btn sm ghost"
                onClick={() => api.updateProject(p.id, { claim_ownership: true })
                  .then(onChanged).catch((e: Error) => onError(e.message))}
              >
                Claim
              </button>
            </Tip>
          )}
          <span className="spacer" />
          <button className="btn sm ghost" onClick={onDelete}>
            <Icon name="trash" size={13} /> Delete
          </button>
        </div>

        {p.owner?.oid === meOID && <span className="xs faint">You own this project.</span>}
      </div>
    </div>
  );
}

function CreateDialog({
  onClose, onCreated,
}: { onClose: () => void; onCreated: (p: api.Project) => void }) {
  const [name, setName] = useState("");
  const [dir, setDir] = useState("");
  const [picking, setPicking] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  // The folder's own name already means something to whoever picked it, so it
  // fills the name field until they type over it.
  const pick = (path: string) => {
    setDir(path);
    setPicking(false);
    setName((cur) => cur.trim() || fileOf(path));
  };

  const submit = (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    api.createProject(name.trim(), dir)
      .then((d) => onCreated(d.project))
      .catch((err: Error) => { setError(err.message); setBusy(false); });
  };

  if (picking) {
    return (
      <DirPicker
        title="Choose the image folder"
        hint="Every image in here goes into the labeling queue."
        onPick={pick}
        onClose={() => setPicking(false)}
      />
    );
  }

  return (
    <div className="scrim" onClick={onClose}>
      <form
        className="modal"
        style={{ maxWidth: 520 }}
        onClick={(e) => e.stopPropagation()}
        onSubmit={submit}
      >
        <div className="modal-head">
          <span className="card-title"><Icon name="plus" size={13} /> New project</span>
        </div>

        <div className="modal-body col" style={{ gap: 14 }}>
          <label className="col" style={{ gap: 6 }}>
            <span className="xs muted">Name</span>
            <input autoFocus required value={name} placeholder="What is this work called?"
              onChange={(e) => setName(e.target.value)} />
          </label>

          <div className="col" style={{ gap: 6 }}>
            <span className="xs muted">Image folder on the server</span>
            <div className="row" style={{ gap: 6 }}>
              <input className="grow input-mono" value={dir} required spellCheck={false}
                placeholder="folder of images to label" onChange={(e) => setDir(e.target.value)} />
              <button type="button" className="btn" onClick={() => setPicking(true)}>
                <Icon name="folder" size={14} /> Browse
              </button>
            </div>
            <span className="xs faint">
              Labels, the taught examples and the held-out test set are all managed for
              you — in the database and a hidden <code>.ctflow</code> folder in here.
            </span>
          </div>

          <div className="col" style={{ gap: 6 }}>
            <span className="xs muted">Type of work</span>
            {/* One value today. A dropdown with a single option promises a choice
                the app cannot honour, so this stays a chip until a second module
                exists to answer for another value. */}
            <span className="chip" style={{ alignSelf: "flex-start" }}>
              <Icon name="target" size={12} /> object detection
            </span>
          </div>

          {/* FR-29 — designed, deliberately inert: there is no answer yet for
              where an uploaded file should land, and guessing one is how a
              dataset ends up somewhere nobody can find. */}
          <Soon why="Needs a decision on where uploaded files land (FR-29).">
            <div className="dropzone" style={{ flexDirection: "row", padding: "12px 14px", textAlign: "left", gap: 12 }}>
              <span style={{ color: "var(--brand)" }}><Icon name="upload" size={16} /></span>
              <span className="col grow" style={{ gap: 1 }}>
                <strong style={{ fontSize: 12.5, color: "var(--text)" }}>…or drag image files here</strong>
                <span className="xs">For people who cannot reach the server&rsquo;s folders.</span>
              </span>
              <button type="button" className="btn sm" disabled>Choose files…</button>
            </div>
          </Soon>

          {error && (
            <div className="note bad" role="alert"><Icon name="alert" size={15} /><span>{error}</span></div>
          )}

          <div className="row between">
            <button type="button" className="btn ghost" onClick={onClose}>Cancel</button>
            <button className="btn primary" disabled={busy || !name.trim() || !dir}>
              {busy ? "Creating…" : "Create and open"}
            </button>
          </div>
        </div>
      </form>
    </div>
  );
}

/** Coarse on purpose: the question a card answers is "how stale is this", and a
 *  wall of timestamps reads worse than "2 days ago". */
function ago(iso: string) {
  const secs = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000);
  const [n, unit] =
    secs < 90 ? [secs, "second"] :
    secs < 5400 ? [secs / 60, "minute"] :
    secs < 129600 ? [secs / 3600, "hour"] :
    [secs / 86400, "day"];
  const r = Math.round(n);
  return `${r} ${unit}${r === 1 ? "" : "s"} ago`;
}
