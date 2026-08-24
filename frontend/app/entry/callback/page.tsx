"use client";

import { useEffect, useRef, useState } from "react";
import { BrandMark } from "../../lib/ui";
import * as api from "../../lib/api";

export default function CallbackPage() {
  const started = useRef(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (started.current) return;
    started.current = true;
    const params = new URLSearchParams(window.location.search);
    const code = params.get("code");
    const state = params.get("state");
    if (!code || !state) {
      setError(params.get("error_description") ?? "Missing login code or state");
      return;
    }
    api.loginCallback(code, state)
      .then(() => window.location.replace("/"))
      .catch((e) => setError(e instanceof Error ? e.message : String(e)));
  }, []);

  return (
    <main className="row" style={{ minHeight: "100dvh", justifyContent: "center", padding: 16 }}>
      <section className="card pad col" style={{ width: "100%", maxWidth: 420, textAlign: "center", gap: 20 }}>
        <div><span className="brand-mark" style={{ margin: "0 auto 12px" }}><BrandMark /></span><h1>Processing Login</h1></div>
        {error ? <><div className="note bad" role="alert">{error}</div><a className="btn" href="/entry/login">Back to login</a></> : <p className="muted">Please wait while we verify your credentials…</p>}
      </section>
    </main>
  );
}
