"use client";

import { useEffect } from "react";
import DirPicker from "./components/DirPicker";
import ProgressBar from "./components/ProgressBar";
import SetupCard from "./components/SetupCard";
import ShortcutsDialog from "./components/ShortcutsDialog";
import { useSession, type Panel } from "./lib/session";
import { BrandMark, fileOf, Icon, pct, Tip, type IconName } from "./lib/ui";
import InsightsPanel from "./panels/InsightsPanel";
import PoolPanel from "./panels/PoolPanel";
import ReportPanel from "./panels/ReportPanel";
import TestsetPanel from "./panels/TestsetPanel";

export default function Page() {
  const s = useSession();

  /** FR-20 — the whole repetitive part of the loop, on keys. Deliberately inert
   *  while a field or a dialog has focus: nothing is worse than a shortcut that
   *  eats the class name you were typing. */
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      if (el && (/^(INPUT|TEXTAREA|SELECT)$/.test(el.tagName) || el.isContentEditable)) return;
      if (s.picking || s.showShortcuts) return;

      const inTestset = s.panel === "testset";
      const stack = inTestset ? s.ts : s.pool;
      const list = inTestset ? s.tsImages : s.sortedPool;
      const cur = inTestset ? s.tsCurrent : s.current;
      const go = inTestset ? s.goToTsImage : s.goToImage;
      const step = (d: number) => {
        if (!list.length) return;
        const i = cur ? list.indexOf(cur) : -1;
        go(list[Math.min(list.length - 1, Math.max(0, i + d))] ?? null);
      };

      if (e.key === "?") { e.preventDefault(); s.setShowShortcuts(true); return; }
      /** Enter saves too. Ctrl+S is the one people expect; Enter is the one
       *  that keeps a hand on the mouse and still finishes an image. Not while
       *  a button has focus -- there it already means "press this button". */
      if (e.key === "Enter" && el?.tagName === "BUTTON") return;
      if (e.key === "Enter" || ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s")) {
        e.preventDefault();
        if (inTestset) s.saveTestset();
        else if (s.isReview) s.saveReview();
        else s.saveLabel();
        return;
      }
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "z") {
        e.preventDefault();
        if (e.shiftKey) stack.redo(); else stack.undo();
        return;
      }
      if (e.ctrlKey || e.metaKey || e.altKey) return;

      if (e.key === "ArrowRight" || e.key.toLowerCase() === "n") { e.preventDefault(); step(1); }
      else if (e.key === "ArrowLeft" || e.key.toLowerCase() === "p") { e.preventDefault(); step(-1); }
      else if (e.key.toLowerCase() === "s") { e.preventDefault(); step(1); }
      else if (e.key === "Escape") { stack.set([]); }
      else if (e.key === "Delete" || e.key === "Backspace") {
        if (s.selected === null) return;
        e.preventDefault();
        const i = s.selected;
        stack.set((cur_) => cur_.filter((_, idx) => idx !== i));
        s.setSelected(null);
      }
      else if (e.key.toLowerCase() === "c" && !inTestset && s.clipboard) {
        e.preventDefault();
        s.pool.set(s.clipboard.boxes);
      }
      else if (e.key.toLowerCase() === "a" && !inTestset && s.drafts.length) {
        e.preventDefault();
        s.acceptDrafts();
      }
      else if (/^[1-9]$/.test(e.key)) {
        const names = inTestset ? s.tsClasses : s.classNames;
        const name = names[Number(e.key) - 1];
        if (name) { e.preventDefault(); (inTestset ? s.setTsCls : s.setCls)(name); }
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [s]);

  const steps: { key: Panel; label: string; icon: IconName; badge?: string; disabled?: boolean; hint?: string }[] = [
    { key: "pool", label: "Label", icon: "image", badge: s.images.length ? `${s.labeled.size + s.auto.size}/${s.images.length}` : undefined },
    { key: "testset", label: "Test set", icon: "target", badge: s.tsImages.length ? `${s.tsLabeled.length}/${s.tsImages.length}` : undefined },
    { key: "report", label: "Report", icon: "chart", badge: s.evalResult ? pct(s.evalResult.overall.f1) : undefined, disabled: !s.evalResult, hint: "Run Evaluate first" },
    { key: "insights", label: "Progress", icon: "spark", badge: s.history.length ? `${s.history.length}` : undefined },
  ];

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

          <nav className="steps" role="tablist" aria-label="Workflow" style={{ marginLeft: 8 }}>
            {steps.map((st) => (
              <button
                key={st.key}
                role="tab"
                className="step"
                aria-selected={s.panel === st.key}
                disabled={st.disabled}
                title={st.disabled ? st.hint : undefined}
                onClick={() => s.setPanel(st.key)}
              >
                <Icon name={st.icon} size={14} />
                {st.label}
                {st.badge && <span className="step-idx" style={{ width: "auto", padding: "0 5px", borderRadius: 9 }}>{st.badge}</span>}
              </button>
            ))}
          </nav>

          <span className="spacer" />

          <div className="row" style={{ gap: 6 }}>
            {/* FR-32 — one switch swaps the vocabulary for people who don't
                need to know what an embedding is. */}
            <Tip text="Swaps the technical wording for plain language, for anyone who doesn't work with models day to day.">
              <label className="check" style={{ paddingRight: 4 }}>
                <input type="checkbox" checked={s.simple} onChange={(e) => s.setSimple(e.target.checked)} />
                Plain language
              </label>
            </Tip>

            <button
              className="btn ghost icon"
              onClick={() => s.setShowSetup((v) => !v)}
              title="Session setup"
              aria-label="Session setup"
            >
              <Icon name="sliders" size={15} />
            </button>
            <button
              className="btn ghost icon"
              onClick={() => s.setShowShortcuts(true)}
              title="Keyboard shortcuts (?)"
              aria-label="Keyboard shortcuts"
            >
              <Icon name="keyboard" size={15} />
            </button>

            {/* FR-30/31 — the slot is designed so the layout is final; sign-in
                itself is not built, and nothing here pretends it is. */}
            <Tip text="Sign-in is not built yet. Until it is, everyone shares one session and labels are not attributed to a person (T-12).">
              <span className="chip" style={{ opacity: .7 }}>
                <Icon name="lock" size={12} /> Not signed in
              </span>
            </Tip>
          </div>
        </div>
      </header>

      <main
        className="col"
        style={{ gap: 14, padding: "14px var(--s5) var(--s6)", maxWidth: 1600, margin: "0 auto" }}
      >
        {s.reachable === false && (
          <div className="note bad">
            <Icon name="alert" size={15} />
            <span><strong>Cannot reach the backend.</strong> The API server is not responding — start it, then reload.</span>
          </div>
        )}

        {(s.showSetup || !s.images.length) && <SetupCard s={s} />}

        {s.progress && <ProgressBar label={s.status} progress={s.progress} />}

        {!s.progress && s.status && (
          <div className="row between wrap" style={{ gap: 10, padding: "0 2px" }}>
            <span className="row xs muted" style={{ gap: 7 }} role="status" aria-live="polite">
              <span className={`dot${s.busy ? " pulse" : ""}`} style={{ color: s.busy ? "var(--brand)" : "var(--faint)" }} />
              {s.status}
            </span>
            <div className="row wrap xs faint" style={{ gap: 10 }}>
              {s.inputDir && <span className="mono truncate" title={s.inputDir} style={{ maxWidth: 240 }}>in: {fileOf(s.inputDir)}</span>}
            </div>
          </div>
        )}

        {s.panel === "pool" && <PoolPanel s={s} />}
        {s.panel === "testset" && <TestsetPanel s={s} />}
        {s.panel === "report" && <ReportPanel s={s} />}
        {s.panel === "insights" && <InsightsPanel s={s} />}
      </main>

      {s.showShortcuts && <ShortcutsDialog onClose={() => s.setShowShortcuts(false)} />}

      {s.picking && (
        <DirPicker
          title="Choose the image folder"
          hint="Every image in here goes into the labeling queue."
          onPick={s.setInputDir}
          onClose={() => s.setPicking(null)}
        />
      )}
    </>
  );
}
