"use client";

import { useEffect, useState } from "react";
import Modal from "./Modal";
import { Empty, Icon } from "../lib/ui";

type Listing = {
  path: string;
  parent: string | null;
  dirs: { name: string; path: string }[];
  images: number;
  roots: string[];
};

/** Browses the *server's* filesystem, confined to LABEL_TOOL_VM_ROOT. There is
 *  no unconfined mode any more (T-27), so what this shows is the whole of what
 *  any path in a request is allowed to reach. */
export default function DirPicker({
  title, hint, onPick, onClose,
}: {
  title: string;
  hint?: string;
  onPick: (path: string) => void;
  onClose: () => void;
}) {
  const [listing, setListing] = useState<Listing | null>(null);
  const [error, setError] = useState("");

  const load = async (path: string) => {
    setError("");
    const res = await fetch(`/api/browse?path=${encodeURIComponent(path)}`);
    if (!res.ok) {
      setError((await res.json()).detail ?? "Cannot open this folder");
      return;
    }
    setListing(await res.json());
  };

  useEffect(() => { load(""); }, []);

  const roots = listing?.roots ?? [];

  return (
    <Modal label={title} width={620} onClose={onClose}>
      <div className="modal-head">
        <div className="col" style={{ gap: 2 }}>
          <h2 style={{ fontSize: 14 }}>{title}</h2>
          {hint && <span className="xs muted">{hint}</span>}
        </div>
        <button className="btn ghost icon" onClick={onClose} aria-label="Close">
          <Icon name="x" size={15} />
        </button>
      </div>

      <div className="modal-body col" style={{ gap: 10, minHeight: 300 }}>
        <div className="row wrap">
          {roots.map((r) => (
            <button key={r} className="btn sm" onClick={() => load(r)}>
              <Icon name="folder" size={13} /> {r}
            </button>
          ))}
        </div>

        <div
          className="mono muted truncate"
          style={{ padding: "7px 10px", background: "var(--bg-deep)", borderRadius: "var(--r-sm)", border: "1px solid var(--line-soft)" }}
        >
          {listing?.path || "Pick a drive or root to start"}
        </div>

        {error && (
          <div className="note bad" role="alert"><Icon name="alert" size={14} /><span>{error}</span></div>
        )}

        {/* Rows are buttons, not clickable divs: this is a file browser, and a
            keyboard user has to be able to walk into a folder. */}
        <div style={{ overflow: "auto", flex: 1, minHeight: 190, maxHeight: "42vh" }}>
          {listing?.parent && (
            <button type="button" className="thumb-row" onClick={() => load(listing.parent!)}>
              <span className="muted" style={{ display: "flex", width: 20, justifyContent: "center" }}>
                <Icon name="arrowLeft" size={14} />
              </span>
              <span className="muted">Up one level</span>
            </button>
          )}
          {listing?.dirs.map((d) => (
            <button type="button" key={d.path} className="thumb-row" onClick={() => load(d.path)}>
              <span style={{ color: "var(--brand)", display: "flex", width: 20, justifyContent: "center" }}>
                <Icon name="folder" size={14} />
              </span>
              <span className="truncate">{d.name}</span>
              <span className="spacer" />
              <span className="faint"><Icon name="chevronRight" size={13} /></span>
            </button>
          ))}
          {listing && listing.dirs.length === 0 && (
            <Empty icon="folder" title="No subfolders here">
              If this is the folder you want, choose it below.
            </Empty>
          )}
        </div>
      </div>

      <div className="modal-foot">
        <span className="xs muted row" style={{ gap: 6 }}>
          {listing && (
            <>
              <Icon name="image" size={13} />
              {listing.images} image{listing.images === 1 ? "" : "s"} directly in this folder
            </>
          )}
        </span>
        <div className="row">
          <button className="btn" onClick={onClose}>Cancel</button>
          <button
            className="btn primary"
            disabled={!listing?.path}
            onClick={() => { onPick(listing!.path); onClose(); }}
          >
            <Icon name="check" size={14} /> Use this folder
          </button>
        </div>
      </div>
    </Modal>
  );
}
