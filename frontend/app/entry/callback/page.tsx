"use client";

import { useEffect, useRef, useState } from "react";
import { BrandMark, useTitle } from "../../lib/ui";
import * as api from "../../lib/api";

export default function CallbackPage() {
  useTitle("Signing in");
  const started = useRef(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (started.current) return;
    started.current = true;
    const params = new URLSearchParams(window.location.search);
    const code = params.get("code");
    if (!code) {
      setError(params.get("error_description") ?? "Missing login code");
      return;
    }
    // The directory hands back no state of its own, so CSRF protection here is
    // a double-submit: /redirect wrote the same random value into an httpOnly
    // cookie AND this sessionStorage entry. A cookie alone proves nothing --
    // it rides along on any cross-site request an attacker forces -- but
    // sessionStorage can only be set by real same-origin JS, which a forced
    // request never runs. Only a browser that genuinely visited /entry/login
    // has both halves to echo back here.
    const state = sessionStorage.getItem("oidc_state") ?? "";
    sessionStorage.removeItem("oidc_state");
    api.loginCallback(code, state)
      .then(() => window.location.replace("/"))
      .catch((e) => setError(e instanceof Error ? e.message : String(e)));
  }, []);

  return (
    <main id="main" className="row" style={{ minHeight: "100dvh", justifyContent: "center", padding: 16 }}>
      <section className="card pad col" style={{ width: "100%", maxWidth: 420, textAlign: "center", gap: 20 }}>
        <div><span className="brand-mark" style={{ margin: "0 auto 12px" }}><BrandMark /></span><h1>Processing Login</h1></div>
        {error ? <><div className="note bad" role="alert">{error}</div><a className="btn" href="/entry/login">Back to login</a></> : <p className="muted">Please wait while we verify your credentials…</p>}
      </section>
    </main>
  );
}
