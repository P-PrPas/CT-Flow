"use client";

import { useEffect, useRef, useState, type FormEvent } from "react";
import Modal from "../../../components/Modal";
import { Icon } from "../../../lib/ui";
import { exportAnnotations, type ExportFormat, type ExportKind } from "../api";

const FORMATS = [
  { id: "yolo", name: "YOLO", extension: "ZIP", detail: "Text labels + classes.txt", filename: "labels_yolo.zip" },
  { id: "coco", name: "COCO", extension: "JSON", detail: "One annotation file", filename: "annotations_coco.json" },
  { id: "voc", name: "Pascal VOC", extension: "ZIP", detail: "One XML file per image", filename: "labels_voc.zip" },
] as const;

export default function ExportDialog({ inputDir, projectName, initialKind, unsaved, onClose }: {
  inputDir: string; projectName: string; initialKind: ExportKind;
  unsaved: { pool: boolean; testset: boolean }; onClose: () => void;
}) {
  const [format, setFormat] = useState<ExportFormat>("yolo");
  const [kind, setKind] = useState<ExportKind>(initialKind);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [started, setStarted] = useState(false);
  const request = useRef<AbortController | null>(null);
  const selected = FORMATS.find((item) => item.id === format)!;

  useEffect(() => () => request.current?.abort(), []);

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (request.current && !request.current.signal.aborted && busy) return;
    const controller = new AbortController();
    request.current = controller;
    setBusy(true); setError(""); setStarted(false);
    try {
      // ponytail: annotation-only payload buffered as a Blob; stream via a
      // server download job if exports grow to include original image files.
      const blob = await exportAnnotations(inputDir, format, kind, controller.signal);
      if (controller.signal.aborted) return;
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url; link.download = selected.filename;
      document.body.append(link); link.click(); link.remove();
      // Give the browser time to acquire the object URL before releasing it.
      window.setTimeout(() => URL.revokeObjectURL(url), 60_000);
      setStarted(true);
    } catch (cause) {
      if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : "Export failed. Please try again.");
    } finally {
      if (!controller.signal.aborted) setBusy(false);
    }
  };

  return (
    <Modal label="Export dataset" width={600} onClose={onClose}>
      <form className="col" style={{ gap: 0, minHeight: 0 }} onSubmit={submit}>
        <div className="modal-head">
          <div className="col" style={{ gap: 4 }}>
            <h2 className="card-title"><Icon name="download" size={18} /> Export dataset</h2>
            <p className="xs muted">{projectName}</p>
          </div>
          <button type="button" className="btn ghost icon" aria-label="Close export" onClick={onClose}><Icon name="x" size={18} /></button>
        </div>
        <div className="modal-body col" style={{ gap: 24 }}>
          <fieldset className="export-fieldset" disabled={busy}>
            <legend>Annotation format</legend>
            <div className="export-formats">
              {FORMATS.map((item) => (
                <label className="export-format" key={item.id}>
                  <span className="row between"><input type="radio" name="export-format" value={item.id} checked={format === item.id} onChange={() => { setFormat(item.id); setError(""); setStarted(false); }} /><span className="xs faint">{item.extension}</span></span>
                  <strong>{item.name}</strong><span className="xs muted">{item.detail}</span>
                </label>
              ))}
            </div>
          </fieldset>
          <label className="col" style={{ gap: 8 }}>
            <strong className="sm">Data source</strong>
            <select value={kind} disabled={busy} aria-describedby="export-source-help" onChange={(event) => { setKind(event.target.value as ExportKind); setError(""); setStarted(false); }}>
              <option value="pool">Pool annotations</option>
              <option value="testset">Test set annotations</option>
            </select>
            <span id="export-source-help" className="xs muted">{kind === "pool" ? "Saved pool annotations, including labels created by your team and the model." : "Saved ground-truth annotations from the test set, with its own class list."}</span>
          </label>
          <div className="note info"><Icon name="info" size={17} /><span><strong>Annotations only.</strong> Original images and train/validation splits are not included. Images with no saved boxes or unreadable source files are omitted.</span></div>
          {unsaved[kind] && <div className="note warn"><Icon name="alert" size={17} /><span>You have unsaved edits in this set. This export uses the last saved annotations. Close this dialog and save first to include your changes.</span></div>}
          <div className="export-file"><Icon name="download" size={18} /><span className="col" style={{ gap: 2 }}><strong className="sm">Your download</strong><span className="mono muted">{selected.filename}</span></span></div>
          {error && <div className="note bad" role="alert"><Icon name="alert" size={17} /><span>{error}</span></div>}
          <p className="xs muted" role="status">{busy ? "Preparing your annotations. You can cancel while the export is running." : started ? "Download started. Check your browser’s downloads." : "Exports use saved annotations and do not change your project."}</p>
        </div>
        <div className="modal-foot">
          <button type="button" className="btn ghost" onClick={onClose}>{busy ? "Cancel export" : "Close"}</button>
          <button type="submit" className="btn primary" disabled={busy}><Icon name="download" size={16} />{busy ? "Preparing…" : error ? "Try again" : "Download annotations"}</button>
        </div>
      </form>
    </Modal>
  );
}
