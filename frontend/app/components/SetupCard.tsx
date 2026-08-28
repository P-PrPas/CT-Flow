"use client";

import ModelPicker from "./ModelPicker";
import type { Session } from "../lib/session";
import { HelpDot, Icon, Soon } from "../lib/ui";

/** Step 0: where the images are and where the work goes. Collapses out of the
 *  way once a session is open -- it is answered once per session, so it should
 *  not hold a permanent third of the screen. */
export default function SetupCard({ s }: { s: Session }) {
  return (
    <div className="card">
      <div className="card-head">
        <span className="card-title"><Icon name="sliders" size={13} /> Session setup</span>
        <div className="row">
          {s.images.length > 0 && (
            <button className="btn ghost sm" onClick={() => s.setShowSetup(false)}>
              <Icon name="x" size={13} /> Hide
            </button>
          )}
        </div>
      </div>

      <div className="card-body col" style={{ gap: 14 }}>
        <div className="col" style={{ gap: 6, maxWidth: 460 }}>
          <label className="row xs muted" style={{ gap: 6, fontWeight: 500 }}>
            Image folder
            <HelpDot text="Every image in here goes into the labeling queue. Labels, the taught examples, and the test set you hold back all live in a hidden .ctflow subfolder inside it — nothing else to pick." />
          </label>
          <div className="row" style={{ gap: 6 }}>
            <input
              className="grow input-mono"
              value={s.inputDir}
              onChange={(e) => s.setInputDir(e.target.value)}
              placeholder="folder of images to label"
              spellCheck={false}
            />
            <button className="btn" onClick={() => s.setPicking("input")} title="Browse folders on the server">
              <Icon name="folder" size={14} /> Browse
            </button>
          </div>
        </div>

        <div style={{ maxWidth: 360 }}>
          <ModelPicker s={s} />
        </div>

        <div className="row wrap between" style={{ gap: 12 }}>
          <div className="row" style={{ gap: 10 }}>
            <button
              className="btn primary"
              onClick={s.openSession}
              disabled={!s.inputDir || s.busy}
            >
              <Icon name="play" size={14} /> Open session
            </button>
          </div>
          {s.images.length > 0 && (
            <span className="chip ok">
              <Icon name="check" size={12} /> {s.images.length} images loaded
            </span>
          )}
        </div>

        {/* FR-29 — designed, deliberately inert: uploading files without the
            auth of T-12 in front of it is not something to ship. */}
        <Soon why="Needs POST /api/upload behind sign-in (T-12 → T-13).">
          <div className="dropzone" style={{ flexDirection: "row", padding: "12px 14px", textAlign: "left", gap: 12 }}>
            <span style={{ color: "var(--brand)" }}><Icon name="upload" size={16} /></span>
            <span className="col grow" style={{ gap: 1 }}>
              <strong style={{ fontSize: 12.5, color: "var(--text)" }}>Drag image files here to upload</strong>
              <span className="xs">
                For people who cannot reach the server&rsquo;s folders — JPG, PNG or BMP.
              </span>
            </span>
            <button className="btn sm" disabled>Choose files…</button>
          </div>
        </Soon>
      </div>
    </div>
  );
}
