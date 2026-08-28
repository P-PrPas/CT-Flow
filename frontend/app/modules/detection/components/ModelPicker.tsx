"use client";

import { useState } from "react";
import Confirm from "../../../components/Confirm";
import type { Session } from "../session";
import type { ModelInfo } from "../types";
import { HelpDot, Icon, Tip } from "../../../lib/ui";

const groupByFamily = (models: ModelInfo[]) =>
  models.reduce<Record<string, ModelInfo[]>>((by, m) => {
    (by[m.family] ??= []).push(m);
    return by;
  }, {});

const modelLabel = (models: ModelInfo[], id: string) => {
  const m = models.find((x) => x.id === id);
  return m ? `${m.family} · ${m.size}` : id;
};

/** Which YOLOE checkpoint a project uses. Free to change anywhere in the app
 *  right up until the bank has its first embedding -- after that every image
 *  in the project has to compare against the same model's embedding space,
 *  so `Bank.lock_model()` fixes it server-side and this collapses to a
 *  read-only chip everywhere it's rendered. The chip isn't a dead end though:
 *  "Switch model…" runs Bank.reembed() -- re-extracts every taught instance
 *  under the new checkpoint and swaps the lock, rather than starting over. */
export default function ModelPicker({ s, label = true }: { s: Session; label?: boolean }) {
  const [switching, setSwitching] = useState(false);
  const [target, setTarget] = useState("");
  const [confirming, setConfirming] = useState(false);

  const current = s.models.find((m) => m.id === (s.bank?.model ?? s.modelId));
  const lockedId = s.bank?.model ?? null;
  const instanceCount = s.bank?.classes.reduce((n, c) => n + c.count, 0) ?? 0;

  const closeSwitcher = () => { setSwitching(false); setTarget(""); };

  return (
    <div className="col" style={{ gap: 6 }}>
      {label && (
        <label className="row xs muted" style={{ gap: 6, fontWeight: 500 }}>
          Model
          <HelpDot text="Bigger sizes are slower but generally more accurate. Locked the moment the first box is saved -- use “Switch model…” to change it in place, or start a new output folder to keep both around. The dot shows whether the weight is already on the server (green) or has to be fetched on first use (red)." />
        </label>
      )}
      {lockedId ? (
        <div className="col" style={{ gap: 6 }}>
          <Tip text="Every embedding in this project's bank was extracted with this model -- they don't mean the same thing to a different checkpoint. Switch model re-extracts all of them under a new one instead of mixing.">
            <span className="chip" style={{ cursor: "help" }}>
              <span className="dot" style={{ color: current?.available ? "var(--ok)" : "var(--bad)" }} />
              <Icon name="lock" size={12} /> {modelLabel(s.models, lockedId)}
            </span>
          </Tip>

          {!switching ? (
            <button
              className="btn ghost sm"
              onClick={() => setSwitching(true)}
              disabled={s.busy}
              title="Re-extract every taught example under a different model"
            >
              <Icon name="refresh" size={12} /> Switch model…
            </button>
          ) : (
            <div className="row wrap" style={{ gap: 6 }}>
              <select
                className="input-mono grow"
                value={target}
                onChange={(e) => setTarget(e.target.value)}
                disabled={s.busy}
              >
                <option value="">choose a model…</option>
                {Object.entries(groupByFamily(s.models.filter((m) => m.id !== lockedId))).map(([family, opts]) => (
                  <optgroup key={family} label={family}>
                    {opts.map((m) => (
                      <option key={m.id} value={m.id}>
                        {m.available ? "🟢" : "🔴"} {m.size}{m.note ? ` — ${m.note}` : ""}
                      </option>
                    ))}
                  </optgroup>
                ))}
              </select>
              <button className="btn primary sm" disabled={!target || s.busy} onClick={() => setConfirming(true)}>
                Go
              </button>
              <button className="btn ghost sm" onClick={closeSwitcher} disabled={s.busy}>
                Cancel
              </button>
            </div>
          )}
        </div>
      ) : (
        <div className="row" style={{ gap: 8, alignItems: "center" }}>
          <span
            className="dot"
            title={current?.available ? "Weight already on the server" : "Not downloaded yet — first use will fetch it"}
            style={{ color: current?.available ? "var(--ok)" : "var(--bad)" }}
          />
          <select
            className="input-mono grow"
            value={s.modelId}
            onChange={(e) => s.setModelId(e.target.value)}
          >
            {Object.entries(groupByFamily(s.models)).map(([family, opts]) => (
              <optgroup key={family} label={family}>
                {opts.map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.available ? "🟢" : "🔴"} {m.size}{m.note ? ` — ${m.note}` : ""}
                  </option>
                ))}
              </optgroup>
            ))}
          </select>
        </div>
      )}

      {confirming && (
        <Confirm
          title="Re-embed with a different model?"
          tone="warn"
          confirmLabel="Re-embed"
          onConfirm={() => { s.reembedModel(target); closeSwitcher(); }}
          onClose={() => setConfirming(false)}
          body={
            <p style={{ margin: 0, fontSize: 13, lineHeight: 1.6 }}>
              Re-runs <strong>{modelLabel(s.models, target)}</strong> over every example already taught in this
              project ({instanceCount} instance{instanceCount === 1 ? "" : "s"}) and replaces the prompt bank's
              vectors with the new model's. Saved label files are untouched — only predict/evaluate/auto-label
              results from here on change, and any cached confidence scores or measured accuracy will need
              re-checking afterward. Can take a while for a large bank.
            </p>
          }
        />
      )}
    </div>
  );
}
