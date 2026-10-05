"use client";

import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { BrandMark, Icon, useTitle } from "../../lib/ui";
import * as api from "../../lib/api";

export default function LoginPage() {
  useTitle("Sign in");
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
      const { redirectUrl, state } = await api.loginRedirect();
      // Written by real same-origin JS right here -- the callback page echoes
      // it back so the backend can tell "this browser started the flow" from
      // "this browser was forced to carry the state cookie" (see callback).
      sessionStorage.setItem("oidc_state", state);
      window.location.assign(redirectUrl);
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
    <main id="main" className="login-page">
      <section className="login-brand" aria-label="About CT-Flow">
        <div className="brand-lockup"><span className="brand-mark"><BrandMark /></span><span className="col" style={{ gap: 4 }}><span className="brand-name">CT-Flow</span><span className="brand-sub">Connected Tech</span></span></div>
      </section>
      <section className="login-content">
        <div className="login-form">
          <div><h1>Sign in</h1></div>
          {auth?.mode === "local" ? (
            <form className="col" onSubmit={localLogin}>
              <label className="col">Username<input autoFocus required autoComplete="username" value={username} placeholder="Enter your username" onChange={(e) => setUsername(e.target.value)} /></label>
              <label className="col">Password<input type="password" required autoComplete="current-password" value={password} placeholder="Enter your password" onChange={(e) => setPassword(e.target.value)} /></label>
              <button className="btn primary block" disabled={loading}>{loading ? "Signing in…" : "Sign in"}<Icon name="arrowRight" size={17} /></button>
            </form>
          ) : (
            <button className="btn primary block" disabled={loading || !auth} onClick={oidcLogin}>{loading ? "Redirecting…" : auth ? "Continue with OAuth" : "Connecting…"}<Icon name="arrowRight" size={17} /></button>
          )}
          {error && <div className="note bad" role="alert"><Icon name="alert" size={16} /><span className="grow">{error}</span>{!auth && <button className="btn sm" onClick={() => window.location.reload()}>Try again</button>}</div>}
          <p className="login-help"><Icon name="lock" size={13} /> Use the account provided by your team. Contact your workspace administrator if you need access.</p>
        </div>
      </section>
    </main>
  );
}
