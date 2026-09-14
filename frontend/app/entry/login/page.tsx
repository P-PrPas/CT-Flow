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
    <main id="main" className="login-page">
      <section className="login-brand" aria-label="About CT-Flow">
        <div className="brand-lockup"><span className="brand-mark"><BrandMark size={24} /></span><span className="col" style={{ gap: 4 }}><span className="brand-name">CT-Flow</span><span className="brand-sub">Connected Tech</span></span></div>
        <div>
          <h2>Human expertise.<br />Machine precision.</h2>
          <p>A shared workspace for labeling images, teaching your model, and building better vision datasets.</p>
          <svg className="login-illustration" viewBox="0 0 440 200" fill="none" aria-hidden="true">
            <rect x="1" y="1" width="438" height="198" rx="14" fill="#1b3639" stroke="#476568" />
            <path d="M1 145h438M1 155h438M1 165h438M1 175h438" stroke="#476568" />
            <path d="M30 145 80 110h285l45 35" stroke="#476568" />
            <rect x="73" y="66" width="62" height="70" rx="5" fill="#436468" />
            <rect x="187" y="54" width="62" height="82" rx="5" fill="#557777" />
            <rect x="302" y="75" width="62" height="61" rx="5" fill="#436468" />
            <path d="M63 80V56h24M121 56h24v24M145 122v24h-24M87 146H63v-24M177 68V44h24M235 44h24v24M259 122v24h-24M201 146h-24v-24M292 89V65h24M350 65h24v24M374 122v24h-24M316 146h-24v-24" stroke="#8ee2d5" strokeWidth="2" />
            <rect x="177" y="21" width="82" height="20" rx="4" fill="#8ee2d5" />
            <text x="218" y="35" textAnchor="middle" fill="#173638" fontSize="11" fontFamily="sans-serif">Component</text>
          </svg>
        </div>
        <span>Made for teams that see the details.</span>
      </section>
      <section className="login-content">
        <div className="login-form">
          <div><span className="eyebrow">Welcome to CT-Flow</span><h1>Welcome back</h1><p>Sign in to continue to your workspace.</p></div>
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
