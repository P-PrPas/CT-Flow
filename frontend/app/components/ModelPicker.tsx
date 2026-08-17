"use client";

import type { Session } from "../lib/session";
import type { ModelInfo } from "../lib/types";
import { HelpDot, Icon, Tip } from "../lib/ui";

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
 *  read-only chip everywhere it's rendered. */
export default function ModelPicker({ s, label = true }: { s: Session; label?: boolean }) {
  const current = s.models.find((m) => m.id === (s.bank?.model ?? s.modelId));

  return (
    <div className="col" style={{ gap: 6 }}>
      {label && (
        <label className="row xs muted" style={{ gap: 6, fontWeight: 500 }}>
          Model
          <HelpDot text="Bigger sizes are slower but generally more accurate. Fixed the moment the first box is saved into an output folder — start a new one to try a different model. The dot shows whether the weight is already on the server (green) or has to be fetched on first use (red)." />
        </label>
      )}
      {s.bank?.model ? (
        <Tip text="This project was already taught with this model — every embedding in its bank has to come out of the same one, so it can't be changed here. Start a new output folder to use a different model.">
          <span className="chip" style={{ cursor: "help" }}>
            <span
              className="dot"
              style={{ color: current?.available ? "var(--ok)" : "var(--bad)" }}
            />
            <Icon name="lock" size={12} /> {modelLabel(s.models, s.bank.model)}
          </span>
        </Tip>
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
    </div>
  );
}
