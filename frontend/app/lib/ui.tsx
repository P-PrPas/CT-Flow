"use client";

/** Shared presentational bits: the SVG icon set, the jargon tooltip, and the
 *  plain-language switch. No emoji icons anywhere -- they render differently
 *  per OS and don't take a color. */

import { useEffect } from "react";
import type { ReactNode } from "react";

// ---------------------------------------------------------------- icons

const PATHS = {
  folder: "M3 7a2 2 0 0 1 2-2h3.9a2 2 0 0 1 1.69.9l.82 1.2H19a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z",
  image: "M3 5h18v14H3zM3 15l5-4 4 3 3-2 6 4",
  target: "M12 3v3M12 18v3M3 12h3M18 12h3M12 7a5 5 0 1 0 0 10 5 5 0 0 0 0-10z",
  brain: "M9 4a3 3 0 0 0-3 3 3 3 0 0 0-1 5.8V17a3 3 0 0 0 4 2.8M15 4a3 3 0 0 1 3 3 3 3 0 0 1 1 5.8V17a3 3 0 0 1-4 2.8M12 4v16",
  chart: "M4 20V10M10 20V4M16 20v-7M22 20H2",
  check: "M20 6 9 17l-5-5",
  x: "M18 6 6 18M6 6l12 12",
  alert: "M12 9v4M12 17h.01M10.3 3.9 1.9 18a2 2 0 0 0 1.7 3h16.8a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z",
  info: "M12 16v-4M12 8h.01M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0z",
  help: "M9.1 9a3 3 0 0 1 5.8 1c0 2-3 3-3 3M12 17h.01M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0z",
  keyboard: "M4 6h16a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1zM7 10h.01M11 10h.01M15 10h.01M17 10h.01M7 14h10",
  undo: "M3 7v6h6M3.5 13a9 9 0 1 0 2.6-7.4L3 8.6",
  redo: "M21 7v6h-6M20.5 13a9 9 0 1 1-2.6-7.4L21 8.6",
  trash: "M3 6h18M8 6V4h8v2M19 6l-1 14H6L5 6M10 11v6M14 11v6",
  copy: "M9 9h10a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H9a1 1 0 0 1-1-1V10a1 1 0 0 1 1-1zM5 15H4a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h10a1 1 0 0 1 1 1v1",
  upload: "M12 16V4M7 9l5-5 5 5M4 17v2a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-2",
  user: "M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2M12 3a4 4 0 1 0 0 8 4 4 0 0 0 0-8z",
  lock: "M5 11h14a1 1 0 0 1 1 1v8a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1v-8a1 1 0 0 1 1-1zM8 11V7a4 4 0 0 1 8 0v4",
  refresh: "M21 12a9 9 0 0 1-15.5 6.2L3 16M3 12a9 9 0 0 1 15.5-6.2L21 8M21 3v5h-5M3 21v-5h5",
  play: "m6 4 14 8-14 8z",
  wand: "m14 4 6 6M3 21l11-11 6 6L9 27zM17 3l.7 1.9L19.6 6l-1.9.7L17 8.6l-.7-1.9L14.4 6l1.9-.7z",
  bot: "M9 11h.01M15 11h.01M12 3v4M7 7h10a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V9a2 2 0 0 1 2-2zM3 13h2M19 13h2",
  layers: "m12 3 9 5-9 5-9-5zM3 13l9 5 9-5M3 17l9 5 9-5",
  gauge: "M12 15a2 2 0 1 0 0-4 2 2 0 0 0 0 4zM13.4 11.6 18 7M21 12a9 9 0 1 0-18 0",
  sliders: "M4 21v-7M4 10V3M12 21v-9M12 8V3M20 21v-5M20 12V3M1 14h6M9 8h6M17 16h6",
  arrowRight: "M5 12h14M13 6l6 6-6 6",
  arrowLeft: "M19 12H5M11 18l-6-6 6-6",
  chevronDown: "m6 9 6 6 6-6",
  chevronRight: "m9 6 6 6-6 6",
  search: "M11 3a8 8 0 1 0 0 16 8 8 0 0 0 0-16zM21 21l-4.3-4.3",
  eye: "M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7-10-7-10-7zM12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z",
  flag: "M4 22V4M4 4h13l-2 4 2 4H4",
  clock: "M12 7v5l3 2M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0z",
  spark: "M12 2v5M12 17v5M4.2 4.2l3.6 3.6M16.2 16.2l3.6 3.6M2 12h5M17 12h5M4.2 19.8l3.6-3.6M16.2 7.8l3.6-3.6",
  save: "M5 3h11l3 3v15a0 0 0 0 1 0 0H5a0 0 0 0 1 0 0zM8 3v6h8V3M8 21v-6h8v6",
  skip: "m5 4 10 8-10 8zM19 5v14",
  plus: "M12 5v14M5 12h14",
  minus: "M5 12h14",
} as const;

export type IconName = keyof typeof PATHS;

export function Icon({ name, size = 15, className }: { name: IconName; size?: number; className?: string }) {
  return (
    <svg
      className={className}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      style={{ flex: "none" }}
    >
      <path d={PATHS[name]} />
    </svg>
  );
}

/** Connected Tech "C" monogram, redrawn as the two-arc mark from the site. */
export function BrandMark({ size = 17 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path d="M15.5 4.6a9 9 0 1 0 0 14.8" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" />
      <path d="M19 7.4a5.4 5.4 0 1 0 0 9.2" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" opacity=".45" />
    </svg>
  );
}

// ---------------------------------------------------------------- tooltip

/** FR-26: every piece of jargon on screen gets a plain-language explanation
 *  within hover/focus reach, so a QC operator never has to ask what a
 *  "prompt bank" is. */
export function Tip({ text, children }: { text: ReactNode; children: ReactNode }) {
  return (
    <span className="tip" tabIndex={0}>
      {children}
      <span className="tip-body" role="tooltip">{text}</span>
    </span>
  );
}

/** A term with a dotted underline that explains itself on hover. */
export function Term({ children, explain }: { children: ReactNode; explain: ReactNode }) {
  return (
    <Tip text={explain}>
      <span className="tip-anchor">{children}</span>
    </Tip>
  );
}

export function HelpDot({ text }: { text: ReactNode }) {
  return (
    <Tip text={text}>
      <span style={{ display: "inline-flex", color: "var(--faint)", cursor: "help" }}>
        <Icon name="help" size={13} />
      </span>
    </Tip>
  );
}

// ---------------------------------------------------------------- misc

/** Per-route document title. Every page in this app is a client component, so
 *  Next's `metadata` export is out of reach and all five would otherwise share
 *  the one title set in layout.tsx (2.4.2). */
export function useTitle(what: string) {
  useEffect(() => { document.title = `${what} · CT-Flow`; }, [what]);
}

export const pct = (v: number) => `${(v * 100).toFixed(1)}%`;
export const fileOf = (p: string) => p.split(/[\\/]/).pop() ?? p;
export const stemOf = (p: string) => fileOf(p).replace(/\.[^.]+$/, "");

/** Above this F1 a class is considered good enough to hand over to auto-label.
 *  Team-agreed number per open question #3 in the requirements doc -- parked
 *  here as one constant so it moves in one place when they settle on it. */
export const READY_F1 = 0.75;

export const gradeColor = (f1: number) =>
  f1 >= READY_F1 ? "var(--ok)" : f1 >= 0.4 ? "var(--warn)" : "var(--bad)";

export function Empty({
  icon, title, children, action,
}: { icon: IconName; title: string; children?: ReactNode; action?: ReactNode }) {
  return (
    <div className="empty">
      <span className="empty-icon"><Icon name={icon} size={20} /></span>
      <h3>{title}</h3>
      {children && <p>{children}</p>}
      {action}
    </div>
  );
}

/** Wraps a designed-but-not-yet-wired feature: visible so the layout is final
 *  and reviewable, inert so nobody thinks it works. */
export function Soon({ children, why }: { children: ReactNode; why: string }) {
  return (
    <div className="soon">
      <div className="soon-inner" aria-disabled="true">{children}</div>
      <div className="xs faint" style={{ marginTop: 6, display: "flex", alignItems: "baseline", gap: 6, flexWrap: "wrap" }}>
        <span className="tag-soon">not built yet</span>
        <span style={{ minWidth: 0 }}>{why}</span>
      </div>
    </div>
  );
}
