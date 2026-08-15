"use client";

import { Icon } from "../lib/ui";

export type JobProgress = { done: number; total: number; startedAt: number; now: number };

/** Progress + ETA for a running job. ETA comes off the server's own clock
 *  (startedAt/now both arrive in the status poll), so it can't be thrown off
 *  by a client whose time is wrong. */
export default function ProgressBar({ label, progress }: { label: string; progress: JobProgress }) {
  const { done, total, startedAt, now } = progress;
  const pct = total > 0 ? Math.min(100, (done / total) * 100) : 0;
  const elapsed = Math.max(0, now - startedAt);
  const rate = elapsed > 0 ? done / elapsed : 0;
  const etaSeconds = rate > 0 && total > done ? (total - done) / rate : null;
  const eta =
    done === 0 ? "estimating…" : etaSeconds === null ? "almost done" : `~${Math.ceil(etaSeconds)}s left`;

  return (
    <div className="card pad col" style={{ gap: 9 }} role="status" aria-live="polite">
      <div className="row between">
        <span className="row" style={{ gap: 7, fontSize: 12.5, fontWeight: 500 }}>
          <span style={{ color: "var(--brand)" }}><Icon name="refresh" size={14} /></span>
          {label}
        </span>
        <span className="muted xs num">
          {done}/{total} · {eta}
        </span>
      </div>
      <div className={`track${done === 0 ? " bar-indeterminate" : ""}`}>
        <span className="fill-brand" style={{ width: `${pct}%` }} />
      </div>
    </div>
  );
}
