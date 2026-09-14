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
import AppShell from "./components/AppShell";
import Confirm from "./components/Confirm";
import DirPicker from "./components/DirPicker";
import Modal from "./components/Modal";
import * as api from "./lib/api";
import { Empty, fileOf, Icon, Tip, useTitle } from "./lib/ui";

/** Where a project of this type is labeled. One entry today; a second module
 *  adds a branch here and a folder, and every card above keeps working. */
const workspaceHref = (p: api.Project) => `/p/${p.id}`;

export default function Home() {
  const router = useRouter();
  useTitle("Projects");
  const [auth, setAuth] = useState<api.AuthState | null>(null);
  const [projects, setProjects] = useState<api.Project[] | null>(null);
  const [error, setError] = useState("");
  const [creating, setCreating] = useState(false);
  const [query, setQuery] = useState("");
  const [scope, setScope] = useState("all");
  const [sort, setSort] = useState("updated");
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
    return <main id="main" role="status" className="row" style={{ minHeight: "100dvh", justifyContent: "center" }}>Loading…</main>;
  }
  const me = auth.user;

  const mine = projects?.filter((p) => p.owner?.oid === auth.oid) ?? [];
  const shown = (projects ?? []).filter((p) =>
    (scope === "all" || (scope === "mine" ? p.owner?.oid === auth.oid : p.owner?.oid !== auth.oid)) &&
    `${p.name} ${p.input_dir} ${p.owner?.username ?? ""}`.toLowerCase().includes(query.trim().toLowerCase())
  ).sort((a, b) => sort === "name" ? a.name.localeCompare(b.name) : Date.parse(b.updated_at) - Date.parse(a.updated_at));
  const totals = projects?.reduce((acc, p) => ({ labeled: acc.labeled + p.labeled, auto: acc.auto + p.auto }), { labeled: 0, auto: 0 });

  return (
    <AppShell user={me} context="Projects" navigation={
      <nav aria-label="Main navigation"><span className="nav-label">Workspace</span><a className="nav-item" href="/" aria-current="page"><Icon name="folder" size={18} /> Projects <span className="nav-count">{projects?.length ?? "—"}</span></a></nav>
    }>
      <main id="main" className="projects-main">
        <div className="page-heading">
          <div><span className="eyebrow">Your workspace</span><h1>Projects</h1><p>Great models start with great data. Pick up where you left off.</p></div>
          <button className="btn primary" onClick={() => setCreating(true)}><Icon name="plus" size={17} /> New project</button>
        </div>

        <div className="overview-grid" aria-label="Workspace overview">
          {[
            { label: "Total projects", value: projects?.length, icon: "folder" as const, hint: "Across your workspace" },
            { label: "Your projects", value: projects ? mine.length : undefined, icon: "user" as const, hint: "Owned by you" },
            { label: "Labeled by hand", value: totals?.labeled, icon: "check" as const, hint: "Images labeled by your team" },
            { label: "Labeled by model", value: totals?.auto, icon: "bot" as const, hint: "Images automatically labeled" },
          ].map((stat) => <div className="overview-stat" key={stat.label}><div className="row between"><span>{stat.label}</span><Icon name={stat.icon} size={18} /></div><strong>{stat.value?.toLocaleString() ?? "—"}</strong><span className="xs faint">{stat.hint}</span></div>)}
        </div>

        <div className="project-toolbar">
          <div className="filter-tabs" role="group" aria-label="Filter projects by owner">
            {[["all", "All projects"], ["mine", "Owned by me"], ["team", "Team projects"]].map(([value, label]) => <button key={value} aria-pressed={scope === value} onClick={() => setScope(value)}>{label}</button>)}
          </div>
          <div className="project-tools">
            <label className="search-field"><Icon name="search" size={17} /><input type="search" aria-label="Search projects" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search projects…" /></label>
            <select aria-label="Sort projects" value={sort} onChange={(e) => setSort(e.target.value)}><option value="updated">Recently updated</option><option value="name">Name A–Z</option></select>
          </div>
        </div>

        {error && (
          <div className="note bad" role="alert">
            <Icon name="alert" size={15} />
            <span className="grow">{error}</span><button className="btn sm" onClick={reload}>Try again</button>
          </div>
        )}

        {projects === null && !error && <div className="project-grid" role="status" aria-label="Loading projects">{[0, 1, 2].map((i) => <div key={i} className="card skeleton-card"><span /><span /><span /></div>)}</div>}

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

        {projects !== null && projects.length > 0 && (
          <Section title={scope === "mine" ? "Your projects" : scope === "team" ? "Team projects" : "All projects"} hint={`${shown.length} project${shown.length === 1 ? "" : "s"}`}>
            {shown.map((p) => <ProjectCard key={p.id} p={p} meOID={auth.oid} onChanged={reload} onDelete={() => setConfirmDelete(p)} onError={setError} />)}
          </Section>
        )}
        {projects && projects.length > 0 && shown.length === 0 && <Empty icon="search" title="No matching projects" action={<button className="btn" onClick={() => { setQuery(""); setScope("all"); }}>Clear filters</button>}>Try a different name, folder, or project owner.</Empty>}
        <footer className="workspace-footer"><span>CT-Flow <span className="faint">/</span> Connected Tech</span><span>Built for human expertise. Powered by machine vision.</span></footer>
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
    </AppShell>
  );
}

function Section({ title, hint, children }: { title: string; hint?: string; children: ReactNode }) {
  return (
    <section className="project-section">
      <div className="row between wrap">
        <h2 style={{ margin: 0, fontSize: 16 }}>{title}</h2>
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
    <article className="card project-card">
      <div className="project-card-top"><span className="project-symbol"><Icon name="folder" size={24} /></span><span className="chip">{p.task_type === "detection" ? "Object detection" : p.task_type}</span></div>
      {renaming ? (
        <form onSubmit={rename} className="row">
          <input className="grow" autoFocus value={name} aria-label="Project name" onChange={(e) => setName(e.target.value)} onKeyDown={(e) => { if (e.key === "Escape") { setName(p.name); setRenaming(false); } }} onBlur={rename} />
        </form>
      ) : <h3><a className="link-title" href={workspaceHref(p)}>{p.name}</a></h3>}
      <span className="project-folder mono" title={p.input_dir}><Icon name="folder" size={13} />{fileOf(p.input_dir)}</span>
      <div className="project-counts">
        <div><strong>{p.labeled.toLocaleString()}</strong><span><Icon name="check" size={13} /> Hand-labeled</span></div>
        <div><strong>{p.auto.toLocaleString()}</strong><span><Icon name="bot" size={13} /> Auto-labeled</span></div>
      </div>
      <div className="project-owner"><span className="avatar small" aria-hidden="true">{p.owner?.username.slice(0, 2).toUpperCase() ?? "—"}</span><span className="grow">{p.owner?.username ?? "Unassigned"}{p.owner?.oid === meOID && <span className="faint"> · You</span>}</span><span className="xs faint">{ago(p.updated_at)}</span></div>
      {p.contributors.length > 0 && <Tip text={p.contributors.map((c) => `${c.username}: ${c.boxes} boxes`).join(" · ")}><span className="xs muted row"><Icon name="layers" size={13} />{p.contributors.length} contributor{p.contributors.length === 1 ? "" : "s"}</span></Tip>}
      <div className="project-card-actions">
        <button className="btn ghost sm" onClick={() => { setName(p.name); setRenaming(true); }}>Rename</button>
        {!p.owner && <button className="btn ghost sm" onClick={() => api.updateProject(p.id, { claim_ownership: true }).then(onChanged).catch((e: Error) => onError(e.message))}>Claim</button>}
        <button className="btn ghost icon sm project-delete" onClick={onDelete} aria-label={`Delete ${p.name}`} title="Delete project"><Icon name="trash" size={15} /></button>
        <span className="spacer" /><a className="btn sm" href={workspaceHref(p)}>Open project <Icon name="arrowRight" size={14} /></a>
      </div>
    </article>
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
    <Modal label="New project" width={520} onClose={onClose}>
      <form onSubmit={submit} className="col" style={{ gap: 0, flex: 1, minHeight: 0 }}>
        <div className="modal-head">
          <h2 className="card-title"><Icon name="plus" size={16} /> New project</h2><button type="button" className="btn ghost icon" aria-label="Close new project" onClick={onClose}><Icon name="x" size={16} /></button>
        </div>

        <div className="modal-body col" style={{ gap: 14 }}>
          <label className="col" style={{ gap: 6 }}>
            <span className="xs muted">Name</span>
            <input autoFocus required value={name} placeholder="What is this work called?"
              onChange={(e) => setName(e.target.value)} />
          </label>

          {/* A real <label>, like the Name field above it. This one only ever
              had a placeholder for a name, and a placeholder leaves the moment
              you type -- on the field the whole project hangs off. */}
          <div className="col" style={{ gap: 6 }}>
            <label className="xs muted" htmlFor="project-dir">Image folder on the server</label>
            <div className="row" style={{ gap: 6 }}>
              <input id="project-dir" aria-describedby="project-dir-help"
                className="grow input-mono" value={dir} required spellCheck={false}
                placeholder="folder of images to label" onChange={(e) => setDir(e.target.value)} />
              <button type="button" className="btn" onClick={() => setPicking(true)}>
                <Icon name="folder" size={14} /> Browse
              </button>
            </div>
            <span className="xs faint" id="project-dir-help">
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
    </Modal>
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
