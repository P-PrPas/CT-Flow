"use client";

/** FR-52 — every image in the project, not just the ones still queued to
 *  label. The Queue card in PoolPanel is "what to label next"; this is "show me
 *  what I have done" and "let me fix that one from three days ago" -- two
 *  different jobs that used to share one 50,000-row scroll box.
 *
 *  Scale is handled without a virtualization library: fixed-size cells with
 *  `content-visibility: auto` (globals.css) let the browser skip layout and
 *  paint for everything off-screen, and IntersectionObserver pages the data in
 *  as the sentinel comes into view. */

import { useCallback, useEffect, useRef, useState } from "react";
import { getPool, thumbUrl, type PoolItem } from "../api";
import type { Session } from "../session";
import { Empty, fileOf, Icon } from "../../../lib/ui";

const PAGE = 200;

type Filter = "all" | "labeled" | "auto" | "unlabeled";

const CHIPS: { key: Filter; label: string }[] = [
  { key: "all", label: "All" },
  { key: "labeled", label: "By hand" },
  { key: "auto", label: "By model" },
  { key: "unlabeled", label: "Unlabeled" },
];

const DOT: Record<PoolItem["status"], string> = {
  labeled: "var(--ok)",
  auto: "var(--brand)",
  unlabeled: "var(--line)",
};

export default function GalleryPanel({ s }: { s: Session }) {
  const [filter, setFilter] = useState<Filter>("all");
  const [items, setItems] = useState<PoolItem[]>([]);
  const [counts, setCounts] = useState<Record<string, number>>({});
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const sentinel = useRef<HTMLDivElement | null>(null);
  /** In-flight guard. A ref rather than the `loading` state because a state
   *  updater has to be pure, and React deliberately calls it twice under
   *  StrictMode to prove it -- running the fetch inside one made `next dev`
   *  request every page twice. */
  const busy = useRef(false);
  const exhausted = items.length >= total;

  /** Reload from the top whenever the filter changes, the folder changes, or a
   *  label lands (s.labeled / s.auto get new references only when their
   *  contents actually change -- see the sameList guard in session.ts), so
   *  coming back from the editor shows the box you just drew. Skipped while the
   *  panel is not on screen: a save on the Label tab must not fire a fetch for
   *  a gallery nobody is looking at. */
  useEffect(() => {
    if (s.panel !== "gallery") return;
    let cancelled = false;
    busy.current = true;
    setLoading(true);
    setError("");
    getPool(s.inputDir, { status: filter, offset: 0, limit: PAGE })
      .then((d) => {
        if (cancelled) return;
        setItems(d.items);
        setCounts(d.counts);
        setTotal(d.total);
      })
      .catch((e: Error) => { if (!cancelled) setError(e.message); })
      .finally(() => { busy.current = false; if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [s.inputDir, s.panel, filter, s.labeled, s.auto]);

  const loadMore = useCallback(() => {
    if (busy.current) return;
    busy.current = true;
    setLoading(true);
    getPool(s.inputDir, { status: filter, offset: items.length, limit: PAGE })
      .then((d) => {
        setItems((cur) => [...cur, ...d.items]);
        setCounts(d.counts);
        setTotal(d.total);
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => { busy.current = false; setLoading(false); });
  }, [s.inputDir, filter, items.length]);

  useEffect(() => {
    const el = sentinel.current;
    if (!el || exhausted) return;
    const io = new IntersectionObserver(
      (entries) => { if (entries[0].isIntersecting) loadMore(); },
      { rootMargin: "800px" }
    );
    io.observe(el);
    return () => io.disconnect();
  }, [loadMore, exhausted]);

  const open = (path: string) => { s.goToImage(path); s.setPanel("pool"); };

  return (
    <div className="col" style={{ gap: 12 }}>
      <div className="row wrap between" style={{ gap: 8 }}>
        <div className="row wrap" style={{ gap: 6 }}>
          {CHIPS.map((c) => (
            <button
              key={c.key}
              className={filter === c.key ? "btn sm primary" : "btn sm"}
              onClick={() => setFilter(c.key)}
            >
              {c.label}
              <span className="chip" style={{ marginLeft: 6 }}>
                {c.key === "all"
                  ? (counts.labeled ?? 0) + (counts.auto ?? 0) + (counts.unlabeled ?? 0)
                  : (counts[c.key] ?? 0)}
              </span>
            </button>
          ))}
        </div>
        <span className="xs faint row" style={{ gap: 5 }}>
          <Icon name="user" size={12} /> green = by hand · cyan = by model · grey = not labeled
        </span>
      </div>

      {error && (
        <div className="note bad"><Icon name="alert" size={14} /><span>{error}</span></div>
      )}

      {!error && !loading && items.length === 0 ? (
        <Empty icon="image" title={filter === "all" ? "No images here" : `No ${filter} images`}>
          {filter === "all"
            ? "The folder this project points at has no images in it."
            : "Nothing has that status yet — label some images or run Auto Label."}
        </Empty>
      ) : (
        <div className="gallery-grid">
          {items.map((it) => (
            <button
              key={it.path}
              className="gallery-cell"
              onClick={() => open(it.path)}
              title={fileOf(it.path)}
            >
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img src={thumbUrl(it.path, 200)} alt="" loading="lazy" width={150} height={112} />
              <span className="gallery-dot" style={{ background: DOT[it.status] }} />
              {it.held_by && (
                <span className="gallery-held">
                  <Icon name="user" size={10} /> {it.held_by}
                </span>
              )}
              <span className="gallery-name">{fileOf(it.path)}</span>
            </button>
          ))}
        </div>
      )}

      <div ref={sentinel} />
      {loading && <div className="xs muted" style={{ padding: 8 }}>Loading…</div>}
      {!loading && exhausted && items.length > 0 && (
        <div className="xs faint" style={{ padding: 8, textAlign: "center" }}>
          {items.length} image{items.length === 1 ? "" : "s"}
        </div>
      )}
    </div>
  );
}
