"use client";

import type { Session } from "../lib/session";
import { HelpDot, Icon, Soon } from "../lib/ui";

/** Step 0: where the images are and where the work goes. Collapses out of the
 *  way once a session is open -- it is answered once per session, so it should
 *  not hold a permanent third of the screen. */
export default function SetupCard({ s }: { s: Session }) {
  const field = (
    key: "input" | "output" | "test",
    label: string,
    value: string,
    set: (v: string) => void,
    placeholder: string,
    help: string
  ) => (
    <div className="col" style={{ gap: 6 }}>
      <label className="row xs muted" style={{ gap: 6, fontWeight: 500 }}>
        {label} <HelpDot text={help} />
      </label>
      <div className="row" style={{ gap: 6 }}>
        <input
          className="grow input-mono"
          value={value}
          onChange={(e) => set(e.target.value)}
          placeholder={placeholder}
          spellCheck={false}
        />
        <button className="btn" onClick={() => s.setPicking(key)} title="Browse folders on the server">
          <Icon name="folder" size={14} /> Browse
        </button>
      </div>
    </div>
  );

  return (
    <div className="card">
      <div className="card-head">
        <span className="card-title"><Icon name="sliders" size={13} /> Session setup</span>
        <div className="row">
          <span className="chip">
            <Icon name="gauge" size={12} />
            {s.mode === "vm" ? "Shared VM" : "This machine"}
          </span>
          {s.images.length > 0 && (
            <button className="btn ghost sm" onClick={() => s.setShowSetup(false)}>
              <Icon name="x" size={13} /> Hide
            </button>
          )}
        </div>
      </div>

      <div className="card-body col" style={{ gap: 14 }}>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(300px, 1fr))", gap: 14 }}>
          {field("input", "Image folder", s.inputDir, s.setInputDir, "folder of images to label",
            "The pool: every image you want labeled. Read-only — nothing is written here.")}
          {field("output", "Output folder", s.outputDir, s.setOutputDir, "where labels and taught examples go",
            "Labels are written here as YOLO .txt files, alongside a _bank folder holding the examples the model has learned from.")}
          {field("test", "Test-set folder", s.testDir, s.setTestDir, "leave blank for <output>/_testset",
            "A small set you label by hand and hold back. It is the only way to measure whether the model is good enough — it never becomes a teaching example. Leave it blank and a _testset folder inside the output folder is used.")}
        </div>

        <div className="row wrap between" style={{ gap: 12 }}>
          <div className="row" style={{ gap: 10 }}>
            <button
              className="btn primary"
              onClick={s.openSession}
              disabled={!s.inputDir || !s.outputDir || s.busy}
            >
              <Icon name="play" size={14} /> Open session
            </button>
            <button className="btn" onClick={s.openTestset} disabled={!s.testDir || s.busy}>
              <Icon name="target" size={14} /> Open test set
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
