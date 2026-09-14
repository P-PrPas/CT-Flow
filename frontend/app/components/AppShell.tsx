"use client";

import { useEffect, useState, type ReactNode } from "react";
import { BrandMark, Icon } from "../lib/ui";
import * as api from "../lib/api";

/** Middlefront's Shell composition, using the existing React primitives. */
export default function AppShell({ user, context, navigation, actions, children }: {
  user: string; context: string; navigation: ReactNode; actions?: ReactNode; children: ReactNode;
}) {
  const [dark, setDark] = useState(false);
  useEffect(() => { setDark(document.documentElement.dataset.theme === "dark"); }, []);

  const toggleTheme = () => {
    const next = !dark;
    setDark(next);
    document.documentElement.dataset.theme = next ? "dark" : "light";
    try { localStorage.setItem("ctflow.theme", next ? "dark" : "light"); } catch { /* Theme still works without storage. */ }
  };

  return (
    <div className="app-shell">
      <aside className="sidebar" aria-label="Workspace navigation">
        <a href="/" className="brand-lockup" aria-label="CT-Flow — all projects">
          <span className="brand-mark"><BrandMark size={24} /></span>
          <span className="col" style={{ gap: 4 }}><span className="brand-name">CT-Flow</span><span className="brand-sub">Connected Tech</span></span>
        </a>
        <div className="sidebar-workspace"><span className="workspace-symbol"><Icon name="layers" size={18} /></span><span className="col" style={{ gap: 0 }}><strong>Team workspace</strong><span className="xs muted">Visual intelligence, together</span></span></div>
        <div className="sidebar-nav">{navigation}</div>
        <div className="sidebar-note"><Icon name="target" size={20} /><strong>Better data. Better vision.</strong><p>Turn your team’s expertise into quality image datasets.</p></div>
        <div className="sidebar-account">
          <span className="avatar" aria-hidden="true">{user.slice(0, 2).toUpperCase()}</span>
          <span className="col grow" style={{ gap: 0 }}><strong className="account-name">{user}</strong><span className="xs muted">Connected workspace</span></span>
          <button className="btn ghost icon" aria-label={`Sign out ${user}`} title="Sign out" onClick={() => api.logout().then((out) => window.location.assign(out.logoutUrl || "/entry/login")).catch(() => window.location.assign("/entry/login"))}><Icon name="logout" size={17} /></button>
        </div>
      </aside>
      <div className="shell-content">
        <header className="topbar">
          <div className="breadcrumb"><a href="/">Workspace</a><Icon name="chevronRight" size={14} /><span>{context}</span></div>
          <div className="topbar-actions">{actions}<button className="btn ghost icon" onClick={toggleTheme} aria-label={dark ? "Use light theme" : "Use dark theme"} title={dark ? "Use light theme" : "Use dark theme"}><Icon name={dark ? "sun" : "moon"} size={18} /></button></div>
        </header>
        {children}
      </div>
    </div>
  );
}
