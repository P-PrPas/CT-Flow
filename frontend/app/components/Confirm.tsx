"use client";

import Modal from "./Modal";
import { Icon, type IconName } from "../lib/ui";

/** An in-app confirm, not window.confirm: it can carry the numbers that make
 *  the decision (FR-27 warns with the actual F1 before a low-accuracy
 *  auto-label pass, which a native dialog can't show). */
export default function Confirm({
  title, tone = "warn", icon = "alert", body, confirmLabel, onConfirm, onClose,
}: {
  title: string;
  tone?: "warn" | "bad" | "info";
  icon?: IconName;
  body: React.ReactNode;
  confirmLabel: string;
  onConfirm: () => void;
  onClose: () => void;
}) {
  return (
    <Modal label={title} role="alertdialog" width={470} onClose={onClose}>
      <div className="modal-head">
        <h2 className="row" style={{ gap: 8, fontSize: 14 }}>
          <span style={{ color: `var(--${tone === "info" ? "brand" : tone})` }}>
            <Icon name={icon} size={16} />
          </span>
          {title}
        </h2>
      </div>
      <div className="modal-body col" style={{ gap: 12 }}>{body}</div>
      <div className="modal-foot">
        {/* Cancel takes the initial focus by being first in the box. The
            confirm button used to claim it with autoFocus, which put a
            destructive default under whatever key was pressed next. */}
        <button className="btn" onClick={onClose}>Cancel</button>
        <button
          className={tone === "info" ? "btn primary" : "btn"}
          style={tone !== "info" ? { borderColor: `var(--${tone})`, color: `var(--${tone})` } : undefined}
          onClick={() => { onConfirm(); onClose(); }}
        >
          {confirmLabel}
        </button>
      </div>
    </Modal>
  );
}
