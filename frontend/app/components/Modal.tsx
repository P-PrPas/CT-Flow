"use client";

import { useEffect, useRef } from "react";
import type { ReactNode } from "react";

/** Every overlay in the app goes through here, and it is a real `<dialog>`
 *  opened with `showModal()`.
 *
 *  Five hand-rolled scrims each had the same three holes: focus walked out of
 *  the box into the page behind it, closing left focus on `<body>`, and the
 *  page behind stayed clickable. `showModal()` closes all three for free --
 *  plus Escape, the top layer, and `::backdrop` in place of `.scrim` -- which
 *  is a smaller amount of code than the focus trap alone would have been.
 *
 *  Two things still have to be wired by hand: `cancel` is preventDefault'd so
 *  React stays the single source of "is this open" (the parent unmounts us),
 *  and a click that lands on the dialog element itself is a backdrop click,
 *  because the backdrop has no node of its own to listen on. */
export default function Modal({
  label, width = 520, role, className = "", onClose, children,
}: {
  /** Accessible name for the dialog. */
  label: string;
  /** Max width in px. The dialog is always narrower than the viewport. */
  width?: number;
  /** "alertdialog" for a decision that interrupts. Defaults to "dialog". */
  role?: "alertdialog";
  /** Extra classes on the dialog element itself. `bare` drops the panel
   *  chrome for overlays that are just a big image. */
  className?: string;
  onClose: () => void;
  children: ReactNode;
}) {
  const ref = useRef<HTMLDialogElement>(null);

  useEffect(() => {
    const d = ref.current;
    // Who to hand focus back to. showModal() restores focus on close(), but
    // this box closes by being unmounted -- the node is gone before the UA
    // ever sees a close -- so the restore is done here instead.
    const opener = document.activeElement as HTMLElement | null;
    if (d && !d.open) d.showModal();
    return () => opener?.focus?.();
  }, []);

  return (
    <dialog
      ref={ref}
      className={`modal ${className}`.trim()}
      role={role}
      aria-label={label}
      style={{ maxWidth: width }}
      onCancel={(e) => { e.preventDefault(); onClose(); }}
      onClick={(e) => { if (e.target === ref.current) onClose(); }}
    >
      {children}
    </dialog>
  );
}
