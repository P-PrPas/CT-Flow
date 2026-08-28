"use client";

import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { BrandMark } from "../../lib/ui";
import * as api from "../../lib/api";

export default function LoginPage() {
  const [auth, setAuth] = useState<api.AuthState | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");

  useEffect(() => {
    api.getAuth()
      .then((state) => state.user ? window.location.replace("/") : setAuth(state))
      .catch((e) => setError(e instanceof Error ? e.message : String(e)));
  }, []);

  const oidcLogin = async () => {
    setLoading(true); setError("");
    try {
      window.location.assign((await api.loginRedirect()).redirectUrl);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e)); setLoading(false);
    }
  };

  const localLogin = async (e: FormEvent) => {
    e.preventDefault(); setLoading(true); setError("");
    try {
      await api.localLogin(username, password); window.location.replace("/");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e)); setLoading(false);
    }
  };

  return (
    <main className="row" style={{ minHeight: "100dvh", justifyContent: "center", padding: 16 }}>
      <section className="card pad col" style={{ width: "100%", maxWidth: 420, gap: 20 }}>
        <div className="row">
          <span className="brand-mark"><BrandMark /></span>
          <div className="col" style={{ gap: 2 }}><h1>Login</h1><span className="muted">Sign in to CT-Flow</span></div>
        </div>
        {auth?.mode === "local" ? (
          <form className="col" onSubmit={localLogin}>
            <label className="col">Username<input autoFocus required value={username} onChange={(e) => setUsername(e.target.value)} /></label>
            <label className="col">Password<input type="password" required value={password} onChange={(e) => setPassword(e.target.value)} /></label>
            <button className="btn primary block" disabled={loading}>Sign in</button>
          </form>
        ) : (
          <button className="btn primary block" disabled={loading || !auth} onClick={oidcLogin}>
            {loading ? "Redirecting…" : "Continue with OAuth"}
          </button>
        )}
        {error && <div className="note bad" role="alert">{error}</div>}
      </section>
    </main>
  );
}
