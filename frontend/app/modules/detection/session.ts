"use client";

/** All labeling state and every action that mutates it. page.tsx owns the
 *  chrome, the panels render slices of this -- so a panel takes one `s` prop
 *  instead of forty. */

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { Box } from "./components/BoxCanvas";
import type { JobProgress } from "../../components/ProgressBar";
import * as api from "./api";
import { adviseAll, appendHistory, clearHistory, loadHistory, type EvalPoint } from "./history";
import type { BankSummary, EvalImage, EvalResult, ModelInfo, Score } from "./types";
import { stemOf } from "../../lib/ui";

export type Panel = "pool" | "gallery" | "testset" | "report" | "insights";

/** Undo/redo over a box list (FR-24). Kept tiny on purpose: a bounded array of
 *  past states is all a 5-box-per-image workflow ever needs. */
type Timeline = { boxes: Box[]; past: Box[][]; future: Box[][] };
const EMPTY: Timeline = { boxes: [], past: [], future: [] };

function useBoxStack(limit = 40) {
  // One state, not three: the three move together, and splitting them means
  // updaters that call each other -- which React re-invokes, so it loops.
  const [t, setT] = useState<Timeline>(EMPTY);

  const set = useCallback((next: Box[] | ((b: Box[]) => Box[])) => {
    setT((s) => ({
      boxes: typeof next === "function" ? next(s.boxes) : next,
      past: [...s.past, s.boxes].slice(-limit),
      future: [],
    }));
  }, [limit]);

  /** Loading a different image is not an edit -- it resets the timeline. */
  const reset = useCallback((next: Box[] = []) => {
    setT(next.length ? { boxes: next, past: [], future: [] } : EMPTY);
  }, []);

  const undo = useCallback(() => {
    setT((s) => s.past.length
      ? {
        boxes: s.past[s.past.length - 1],
        past: s.past.slice(0, -1),
        future: [s.boxes, ...s.future].slice(0, limit),
      }
      : s);
  }, [limit]);

  const redo = useCallback(() => {
    setT((s) => s.future.length
      ? {
        boxes: s.future[0],
        past: [...s.past, s.boxes].slice(-limit),
        future: s.future.slice(1),
      }
      : s);
  }, [limit]);

  return {
    boxes: t.boxes, set, reset, undo, redo,
    canUndo: t.past.length > 0, canRedo: t.future.length > 0,
  };
}

/** Polling hands back a fresh object every time, whether or not anything
 *  changed -- and to React a new reference *is* a change. Every useMemo and
 *  useEffect keyed off that state then re-runs, which is how a 15-second poll
 *  becomes a request per render: claims -> heldByOthers -> nextTodo -> the
 *  claim effect -> POST /api/claim -> claims again, as fast as the round trip
 *  allows. Keeping the old reference when the content matches is what breaks
 *  the cycle, so these two compare before they set.
 *
 *  Order-sensitive on purpose, because both lists arrive ordered: images by
 *  `ORDER BY path`, test-set stems sorted by the handler. */
const sameList = (a: string[], b: string[]) =>
  a.length === b.length && a.every((v, i) => v === b[i]);

const sameClaims = (a: Record<string, string>, b: Record<string, string>) => {
  const keys = Object.keys(a);
  return keys.length === Object.keys(b).length && keys.every((k) => a[k] === b[k]);
};

/** Sum of absolute differences between two 8x8 thumbnails (FR-18). */
const distance = (a: number[], b: number[]) =>
  a.length && a.length === b.length
    ? a.reduce((n, v, i) => n + Math.abs(v - b[i]), 0) / a.length
    : Infinity;

/** `inputDir` comes from the route now, not from this hook.
 *
 *  It used to be typed in and remembered in localStorage, which meant the one
 *  thing identifying your work lived in one browser: a different machine, or a
 *  cleared cache, and you were typing a server path again. /p/{id} resolves the
 *  project and hands the folder down, so the URL is what remembers -- and it is
 *  shareable, which localStorage never was.
 *
 *  `me` is the signed-in subject, not the display name: claims come back named
 *  for reading, but "is this mine" is a comparison, and comparing display names
 *  breaks the day two people share one or someone gets renamed. */
export function useSession(inputDir: string, me: string) {
  // --- environment ------------------------------------------------------
  const [colors, setColors] = useState<string[]>([]);
  const [reachable, setReachable] = useState<boolean | null>(null);

  // --- model choice -------------------------------------------------------
  // Which YOLOE checkpoint a *new* project starts with. Once a bank has any
  // embedding it's locked server-side (Bank.lock_model) -- bank.model below
  // is the source of truth from then on, this is only the picker's value.
  const [models, setModels] = useState<ModelInfo[]>([]);
  const [modelId, setModelId] = useState("");

  // --- shell ------------------------------------------------------------
  const [panel, setPanel] = useState<Panel>("pool");
  const [simple, setSimple] = useState(false);
  const [status, setStatus] = useState("");
  const [busy, setBusy] = useState(false);
  const [progress, setProgress] = useState<JobProgress | null>(null);
  const [showShortcuts, setShowShortcuts] = useState(false);

  // --- session config ---------------------------------------------------
  const [conf, setConf] = useState(0.25);
  // Read by the pre-annotation effect without being a dependency of it --
  // dragging the threshold slider must not fire a predict per step.
  const confRef = useRef(conf);
  confRef.current = conf;

  // --- pool -------------------------------------------------------------
  const [images, setImages] = useState<string[]>([]);
  const [bank, setBank] = useState<BankSummary | null>(null);
  const [scores, setScores] = useState<Record<string, Score>>({});
  const [current, setCurrent] = useState<string | null>(null);
  const [savedBoxes, setSavedBoxes] = useState<Box[]>([]);
  const [cls, setCls] = useState("item");
  const [updateMode, setUpdateMode] = useState(false);
  const pool = useBoxStack();

  /** FR-19: the model's guesses for the image on screen, waiting to be accepted
   *  or ignored. Never saved on their own -- Save only ever sends pool.boxes. */
  const [drafts, setDrafts] = useState<Box[]>([]);
  const [drafting, setDrafting] = useState(false);
  const [preAnnotate, setPreAnnotate] = useState(true);

  /** FR-18: spread picks across different-looking images instead of walking
   *  down a run of near-identical conveyor frames. */
  const [spread, setSpread] = useState(true);

  /** FR-28: which images auto-label left empty, not just how many. */
  const [noDetection, setNoDetection] = useState<string[]>([]);

  /** Labels saved since the last Re-check, so the queue can admit it is stale. */
  const [staleScores, setStaleScores] = useState(0);

  /** FR-48/FR-49: what the other person is doing. `claims` is image path ->
   *  their name; `heldByOthers` is the same thing as a set, which is what the
   *  queue actually consults. */
  const [claims, setClaims] = useState<Record<string, string>>({});
  const [claimNote, setClaimNote] = useState("");

  /** FR-51: the bank was taught things this database has no record of. */
  const [bankOrphaned, setBankOrphaned] = useState(false);

  /** FR-50: who drew the boxes on the image currently open. */
  const [labeledBy, setLabeledBy] = useState<string[]>([]);

  /** §7 success metrics, this session only. ponytail: in memory, so a reload
   *  starts the stopwatch over -- enough to answer "is this faster than doing
   *  it by hand" without building an events table nobody asked for yet. */
  const openedAt = useRef(0);
  const imageAt = useRef(0);
  const [labelSecs, setLabelSecs] = useState<number[]>([]);
  const [reviewed, setReviewed] = useState(0);
  const [firstAutoSecs, setFirstAutoSecs] = useState<number | null>(null);

  useEffect(() => { imageAt.current = Date.now(); }, [current]);

  /** Which box is selected on whichever canvas is showing. Lives here, not in
   *  the panel, so the global Delete shortcut can reach it. */
  const [selected, setSelected] = useState<number | null>(null);

  /** FR-22: the last box set the user committed, so it can be stamped onto the
   *  next frame -- conveyor images differ by a few pixels, not a whole scene. */
  const [clipboard, setClipboard] = useState<{ from: string; boxes: Box[] } | null>(null);

  // --- eval -------------------------------------------------------------
  const [evalResult, setEvalResult] = useState<EvalResult | null>(null);
  const [history, setHistory] = useState<EvalPoint[]>([]);
  const [zoomed, setZoomed] = useState<EvalImage | null>(null);

  // --- test set (parallel flow: writes ground truth, never touches the bank) --
  const [tsImages, setTsImages] = useState<string[]>([]);
  const [tsLabeled, setTsLabeled] = useState<string[]>([]);
  const [tsClasses, setTsClasses] = useState<string[]>([]);
  const [tsCurrent, setTsCurrent] = useState<string | null>(null);
  const [tsSavedBoxes, setTsSavedBoxes] = useState<Box[]>([]);
  const [tsCls, setTsCls] = useState("item");
  const [tsUpdateMode, setTsUpdateMode] = useState(false);
  const [poolPick, setPoolPick] = useState<Set<string>>(new Set());
  const [tsPick, setTsPick] = useState<Set<string>>(new Set());
  const [sampleN, setSampleN] = useState(15);
  const ts = useBoxStack();

  const tsSet = ts.set;
  const tsReset = ts.reset;
  const poolReset = pool.reset;

  useEffect(() => {
    api.getConfig()
      .then((c) => {
        setColors(c.colors); setReachable(true);
        setModels(c.models); setModelId(c.default_model);
      })
      .catch(() => { setReachable(false); setStatus("Backend not reachable — is the API running?"); });
  }, []);

  useEffect(() => {
    let cancelled = false;
    loadHistory(inputDir).then((h) => { if (!cancelled) setHistory(h); });
    return () => { cancelled = true; };
  }, [inputDir]);

  // --- derived ----------------------------------------------------------
  const classNames = useMemo(() => bank?.classes.map((c) => c.name) ?? [], [bank]);
  const labeled = useMemo(() => new Set(bank?.labeled ?? []), [bank]);
  const auto = useMemo(() => new Set(bank?.auto ?? []), [bank]);
  /** Test-flagged images never enter the pool queue -- they're the same file
   *  as a pool image, so offering them here would let a "Save" silently
   *  break the held-out invariant the backend otherwise enforces. */
  const testFlagged = useMemo(() => new Set(tsImages), [tsImages]);
  const remaining = useMemo(
    () => images.filter((p) => !labeled.has(p) && !auto.has(p) && !testFlagged.has(p)),
    [images, labeled, auto, testFlagged]
  );
  const bankTotal = useMemo(
    () => bank?.classes.reduce((n, c) => n + c.count, 0) ?? 0,
    [bank]
  );
  const promptCounts = useMemo(
    () => Object.fromEntries((bank?.classes ?? []).map((c) => [c.name, c.count])),
    [bank]
  );
  const advice = useMemo(
    () => adviseAll(classNames, history),
    [classNames, history]
  );

  const isReview = current !== null && auto.has(current);

  const color = useCallback((name: string) => {
    const i = classNames.indexOf(name);
    return colors[(i < 0 ? classNames.length : i) % (colors.length || 1)] ?? "#08d9d6";
  }, [classNames, colors]);

  const tsColor = useCallback((name: string) => {
    const i = tsClasses.indexOf(name);
    return colors[(i < 0 ? tsClasses.length : i) % (colors.length || 1)] ?? "#08d9d6";
  }, [tsClasses, colors]);

  const tsLabeledSet = useMemo(() => new Set(tsLabeled), [tsLabeled]);
  /** Test images are pool images by path now (no copy, no filename dance) --
   *  a pool image already flagged is not a candidate to flag again. */
  const poolCandidates = useMemo(
    () => images.filter((p) => !testFlagged.has(p)),
    [images, testFlagged]
  );

  /** Least-confident-first: the pool order is the tool's opinion about which
   *  image is worth a human minute next. Done images sink to the bottom. */
  /** Images another person is on right now. Yours is not in here -- holding an
   *  image must not hide it from you. */
  const heldByOthers = useMemo(() => {
    const out = new Set<string>();
    for (const [path, who] of Object.entries(claims)) if (who !== me) out.add(path);
    return out;
  }, [claims, me]);

  const sortedPool = useMemo(() => {
    const isDone = (p: string) => labeled.has(p) || auto.has(p);
    const todo = images
      .filter((p) => !isDone(p) && !testFlagged.has(p))
      .sort((a, b) => (scores[a]?.conf ?? -1) - (scores[b]?.conf ?? -1));
    const done = images.filter((p) => isDone(p) && !testFlagged.has(p));
    if (!spread || todo.length < 3) return [...todo, ...done];

    // FR-18 — greedy: out of the few least-confident images still queued, take
    // the one that looks least like the ones just queued. A run of identical
    // conveyor frames should not become a run of identical labeling jobs.
    // Falls back to plain confidence order when nothing has been scored yet.
    const WINDOW = 8, RECENT = 3;
    const left = [...todo];
    const out: string[] = [];
    while (left.length) {
      let pick = 0, far = -1;
      left.slice(0, WINDOW).forEach((p, i) => {
        const sig = scores[p]?.sig ?? [];
        const d = out.length
          ? Math.min(...out.slice(-RECENT).map((q) => distance(sig, scores[q]?.sig ?? [])))
          : Infinity;
        if (d > far) { far = d; pick = i; }
      });
      out.push(left.splice(pick, 1)[0]);
    }
    return [...out, ...done];
  }, [images, scores, labeled, auto, spread, testFlagged]);

  /** The next image worth opening, in the order the queue is showing. `done`
   *  is passed in rather than read from state so callers can use the bank they
   *  just got back instead of the one React has not re-rendered with yet. */
  /** FR-49 -- the whole point of claims: "the next image to label" must differ
   *  between two people in the same project. Falls back to a held image only if
   *  every remaining one is held, because handing back nothing would be worse
   *  than handing back an image someone is already on. */
  const nextTodo = useCallback(
    (done: Set<string>, exclude?: string | null) => {
      const free = sortedPool.find(
        (p) => p !== exclude && !done.has(p) && !heldByOthers.has(p));
      return free ?? sortedPool.find((p) => p !== exclude && !done.has(p)) ?? null;
    },
    [sortedPool, heldByOthers]
  );

  const doneSet = useMemo(() => new Set([...labeled, ...auto]), [labeled, auto]);

  // --- saved-box loading -------------------------------------------------
  useEffect(() => {
    if (!current || !inputDir || !(labeled.has(current) || auto.has(current))) {
      setSavedBoxes(EMPTY.boxes);
      setLabeledBy([]);
      return;
    }
    const reviewing = auto.has(current);
    let cancelled = false;
    api.getBoxes(inputDir, current)
      .then((d) => {
        if (cancelled) return;
        // Auto-labeled images open *in* review mode: the model's boxes land in
        // the editable set so the job is correcting, not redrawing.
        if (reviewing) { poolReset(d.boxes ?? []); setSavedBoxes([]); }
        else setSavedBoxes(d.boxes ?? []);
        setLabeledBy((d.labeled_by ?? []).map((u) => u.username));
      })
      .catch(() => { if (!cancelled) { setSavedBoxes([]); setLabeledBy([]); } });
    return () => { cancelled = true; };
  }, [current, inputDir, labeled, auto, poolReset]);

  /** FR-19 / T-05 — ask the model what it thinks is in this image while the
   *  user is still looking at it. Fired only for images nobody has labeled and
   *  only once the bank has something to prompt with, so an empty bank costs
   *  nothing. Failures are silent: a missing suggestion is not an error, it
   *  just means drawing by hand, which is what the tool did yesterday. */
  useEffect(() => {
    setDrafts([]);
    if (!preAnnotate || !current || !inputDir || !classNames.length) return;
    if (doneSet.has(current)) return;
    let cancelled = false;
    setDrafting(true);
    api.predict(inputDir, current, confRef.current)
      .then((d) => { if (!cancelled) setDrafts(d.boxes.map((b) => ({ cls: b.cls, box: b.box }))); })
      .catch(() => { /* silent by design */ })
      .finally(() => { if (!cancelled) setDrafting(false); });
    return () => { cancelled = true; };
  }, [current, inputDir, preAnnotate, classNames, doneSet]);

  useEffect(() => {
    if (!tsCurrent || !inputDir || !tsLabeledSet.has(stemOf(tsCurrent))) {
      setTsSavedBoxes(EMPTY.boxes);
      return;
    }
    let cancelled = false;
    api.getBoxes(inputDir, tsCurrent, "test")
      .then((d) => { if (!cancelled) setTsSavedBoxes(d.boxes ?? []); })
      .catch(() => { if (!cancelled) setTsSavedBoxes([]); });
    return () => { cancelled = true; };
  }, [tsCurrent, inputDir, tsLabeledSet]);

  // --- actions ----------------------------------------------------------
  const guard = useCallback(async (label: string, fn: () => Promise<void>) => {
    setBusy(true);
    setStatus(label);
    try {
      await fn();
    } catch (e) {
      setStatus(e instanceof Error ? e.message : String(e));
    }
    setBusy(false);
    setProgress(null);
  }, []);

  const openSession = useCallback(() =>
    guard("Opening session…", async () => {
      // One folder in, everything back in one shot -- the bank and the
      // test-set manifest both live under it server-side (see
      // backend/deps.py), so there's no second "did you forget the test set"
      // request to make and nothing here can go stale relative to the other.
      const d = await api.openSession(inputDir);
      setImages(d.images);
      setBank(d.bank);
      setBankOrphaned(d.bank_orphaned);
      // A project that already has embeddings is already locked to a model --
      // reflect that instead of whatever the picker last happened to show.
      if (d.bank.model) setModelId(d.bank.model);
      setScores({});
      poolReset([]);
      setEvalResult(null);
      setClipboard(null);
      setNoDetection([]);
      setStaleScores(0);
      openedAt.current = Date.now();
      setLabelSecs([]);
      setReviewed(0);
      setFirstAutoSecs(null);
      const done = new Set<string>([...d.bank.labeled, ...d.bank.auto]);
      setCurrent(d.images.find((p) => !done.has(p)) ?? d.images[0] ?? null);
      setPanel("pool");
      setStatus(`${d.images.length} image(s) · ${d.bank.labeled.length} labeled by hand`);

      setTsImages(d.testset.images);
      setTsLabeled(d.testset.labeled);
      setTsClasses(d.testset.classes);
      const tsDone = new Set<string>(d.testset.labeled);
      setTsCurrent(d.testset.images.find((p) => !tsDone.has(stemOf(p))) ?? d.testset.images[0] ?? null);
    }), [inputDir, guard, poolReset]);

  /** Opening is no longer a button. The route already resolved which project
   *  this is, so asking the user to confirm the folder they just clicked would
   *  be a step that answers nothing. */
  useEffect(() => { if (inputDir) openSession(); }, [inputDir, openSession]);

  /** FR-48 -- someone else's progress, without a page refresh.
   *
   *  Every 15 seconds, and only while the tab is actually being looked at: a
   *  workspace left open overnight should cost nothing. Polling rather than a
   *  socket because the thing being watched changes every few minutes at most,
   *  and a socket is a connection to keep alive, reconnect and scale.
   *
   *  Failures are silent on purpose -- a missed poll means slightly stale
   *  numbers, which is the state the tool was in permanently until now. */
  useEffect(() => {
    if (!inputDir) return;
    let cancelled = false;
    const pull = () => {
      if (document.visibilityState !== "visible") return;
      api.getState(inputDir)
        .then((st) => {
          if (cancelled) return;
          setClaims((cur) => (sameClaims(cur, st.claims) ? cur : st.claims));
          setBank((cur) =>
            !cur || (sameList(cur.labeled, st.labeled) && sameList(cur.auto, st.auto))
              ? cur
              : { ...cur, labeled: st.labeled, auto: st.auto });
          setTsLabeled((cur) => (sameList(cur, st.testset_labeled) ? cur : st.testset_labeled));
        })
        .catch(() => { /* stale numbers, not an error */ });
    };
    const timer = setInterval(pull, 15_000);
    document.addEventListener("visibilitychange", pull);
    return () => {
      cancelled = true;
      clearInterval(timer);
      document.removeEventListener("visibilitychange", pull);
    };
  }, [inputDir]);

  /** FR-49 -- say which image is being worked on, so the other queue moves off
   *  it. Re-sent every 5 minutes because a claim expires in ten: this is the
   *  heartbeat, and re-claiming your own image renews rather than conflicts.
   *
   *  A 409 means someone got there first, so move on rather than sit on an
   *  image two people are drawing. Only for images nobody has labeled -- there
   *  is nothing to collide over on a finished one. */
  useEffect(() => {
    if (!inputDir || !current || doneSet.has(current)) return;
    let cancelled = false;
    const take = () =>
      api.claimImage(inputDir, current)
        .then((d) => {
          if (cancelled) return;
          setClaims((cur) => (sameClaims(cur, d.claims) ? cur : d.claims));
          setClaimNote("");
        })
        .catch((e: Error) => {
          if (cancelled) return;
          setClaimNote(e.message);
          setCurrent(nextTodo(doneSet, current));
        });
    take();
    const timer = setInterval(take, 5 * 60_000);
    return () => { cancelled = true; clearInterval(timer); };
  }, [inputDir, current, doneSet, nextTodo]);

  /** FR-19 — take the model's guesses into the editable set. Nothing reaches a
   *  label file until this happens, so a suggestion is never silently saved. */
  const acceptDrafts = useCallback(() => {
    if (!drafts.length) return;
    pool.set((cur) => [...cur, ...drafts]);
    setDrafts([]);
  }, [drafts, pool]);

  const goToImage = useCallback((p: string | null) => {
    poolReset([]);
    setCurrent(p);
  }, [poolReset]);

  const saveLabel = () =>
    guard("Extracting visual prompt…", async () => {
      if (!current || !pool.boxes.length) return;
      const saved = pool.boxes;
      const d = await api.saveLabel(inputDir, current, saved, updateMode ? "update" : "replace", modelId);
      setBank(d.bank);
      if (d.bank.model) setModelId(d.bank.model);
      setClipboard({ from: current, boxes: saved });
      poolReset([]);
      setStaleScores((n) => n + 1);
      setLabelSecs((t) => [...t, (Date.now() - imageAt.current) / 1000]);
      setCurrent(nextTodo(new Set<string>([...d.bank.labeled, ...d.bank.auto]), current));
      setStatus(`Saved — ${d.bank.classes.reduce((n, c) => n + c.count, 0)} example(s) taught so far`);
    });

  /** Review edits go through /api/relabel: rewriting a label file, no embedding
   *  extraction. Deleting a wrong prediction isn't a new visual prompt. */
  const saveReview = () =>
    guard("Saving corrections…", async () => {
      if (!current) return;
      const kept = pool.boxes.length;
      const d = await api.relabel(inputDir, current, pool.boxes, updateMode ? "update" : "replace");
      setBank(d.bank);
      poolReset([]);
      setReviewed((n) => n + 1);
      const autoSet = new Set<string>(d.bank.auto);
      const nextAuto = images.find((p) => p !== current && autoSet.has(p));
      setCurrent(nextAuto ?? nextTodo(new Set<string>([...d.bank.labeled, ...d.bank.auto]), current));
      setStatus(`Corrections saved — ${kept} box(es) kept`);
    });

  /** FR-25: the same corrected boxes, but through /api/label so the model
   *  actually learns from them. The difference between this and Save
   *  corrections is the whole reason review work can feel wasted. */
  const teachFromReview = () =>
    guard("Teaching the model from your corrections…", async () => {
      if (!current || !pool.boxes.length) return;
      const saved = pool.boxes;
      const d = await api.saveLabel(inputDir, current, saved, "replace", modelId);
      setBank(d.bank);
      setClipboard({ from: current, boxes: saved });
      poolReset([]);
      setStaleScores((n) => n + 1);
      setLabelSecs((t) => [...t, (Date.now() - imageAt.current) / 1000]);
      const autoSet = new Set<string>(d.bank.auto);
      setCurrent(images.find((p) => p !== current && autoSet.has(p))
        ?? nextTodo(new Set<string>([...d.bank.labeled, ...d.bank.auto]), current));
      setStatus("Corrections saved and added to the model's examples");
    });

  const rescore = () =>
    guard("Re-checking the remaining images…", async () => {
      const r = await api.rescorePool(inputDir, remaining, setProgress);
      setScores((cur) => ({ ...cur, ...r.scores }));
      setStaleScores(0);
      setStatus(`Re-checked ${Object.keys(r.scores).length} image(s)`);
    });

  const runEval = () =>
    guard("Measuring accuracy on the test set…", async () => {
      const r = await api.evaluateTestSet(inputDir, conf, setProgress);
      setEvalResult(r);
      setZoomed(null);
      setHistory(await appendHistory(inputDir, r, promptCounts));
      setStatus(`Test set: F1 ${(r.overall.f1 * 100).toFixed(1)}% over ${r.images} image(s)`);
    });

  const runAuto = () =>
    guard("Labeling the rest…", async () => {
      const d = await api.autoLabelRemaining(inputDir, remaining, conf, setProgress);
      setBank(d.bank);
      setNoDetection(d.no_detection_images ?? []);
      if (firstAutoSecs === null && openedAt.current) {
        setFirstAutoSecs((Date.now() - openedAt.current) / 1000);
      }
      setStatus(`Auto-labeled ${d.written} image(s) · ${d.no_detection} with nothing found`);
    });

  const resetHistory = () => clearHistory(inputDir).then(setHistory);

  /** The only sanctioned way to change a locked project's model -- re-runs
   *  the new checkpoint over every taught instance and swaps the bank's
   *  vectors + lock atomically (Bank.reembed). Everything downstream of the
   *  old model is now stale: cached confidence scores, any measured F1, and
   *  whatever drafts are on screen for the current image. */
  const reembedModel = (newModelId: string) =>
    guard(`Switching to ${newModelId}…`, async () => {
      const d = await api.reembedBank(inputDir, newModelId, setProgress);
      setBank(d.bank);
      setModelId(newModelId);
      setDrafts([]);
      setScores({});
      setStaleScores(0);
      setEvalResult(null);
      setStatus(`Switched to ${newModelId} — re-check the pool and re-evaluate when ready`);
    });

  // --- test-set actions ---------------------------------------------------
  // No separate "open" -- openSession above already bundled this state, and
  // every action below returns its own fresh copy of it.
  const importToTestset = (paths: string[]) =>
    guard("Flagging as test images…", async () => {
      if (!inputDir || !paths.length) return;
      const d = await api.importTestset(inputDir, paths);
      setTsImages(d.images);
      setTsLabeled(d.labeled);
      setTsClasses(d.classes);
      setPoolPick(new Set());
      if (!tsCurrent) setTsCurrent(d.images[0] ?? null);
      setStatus(`Flagged ${d.imported.length} image(s) as test set`);
    });

  const addRandomFromPool = () =>
    importToTestset([...poolCandidates].sort(() => Math.random() - 0.5).slice(0, sampleN));

  const saveTestset = () =>
    guard("Writing ground truth…", async () => {
      if (!tsCurrent || !ts.boxes.length) return;
      const d = await api.labelTestset(inputDir, tsCurrent, ts.boxes, tsUpdateMode ? "update" : "replace");
      setTsClasses(d.classes);
      setTsLabeled(d.labeled);
      tsReset([]);
      const done = new Set<string>(d.labeled);
      setTsCurrent(tsImages.filter((p) => !done.has(stemOf(p)))[0] ?? null);
      setStatus(`Ground truth saved — ${d.labeled.length}/${tsImages.length} done`);
    });

  const removeFromTestset = (paths: string[]) =>
    guard("Removing from the test set…", async () => {
      if (!inputDir || !paths.length) return;
      const d = await api.removeTestset(inputDir, paths);
      setTsImages(d.images);
      setTsLabeled(d.labeled);
      setTsClasses(d.classes);
      setTsPick(new Set());
      if (tsCurrent && paths.includes(tsCurrent)) {
        tsReset([]);
        setTsCurrent(d.images[0] ?? null);
      }
      setStatus(`Removed ${d.removed.length} image(s)`);
    });

  const goToTsImage = useCallback((p: string | null) => {
    tsReset([]);
    setTsCurrent(p);
  }, [tsReset]);

  return {
    // env + shell
    colors, reachable, panel, setPanel, simple, setSimple,
    status, setStatus, busy, progress, showShortcuts, setShowShortcuts,
    claims, heldByOthers, claimNote, bankOrphaned, labeledBy,
    // config
    inputDir, conf, setConf,
    models, modelId, setModelId, reembedModel,
    // pool
    images, bank, scores, current, savedBoxes, cls, setCls, updateMode, setUpdateMode, selected, setSelected,
    pool, clipboard, setClipboard, classNames, labeled, auto, remaining, bankTotal,
    promptCounts, isReview, color, sortedPool, goToImage, nextTodo, doneSet,
    drafts, drafting, acceptDrafts, setDrafts, preAnnotate, setPreAnnotate,
    spread, setSpread, noDetection, staleScores,
    labelSecs, reviewed, firstAutoSecs,
    openSession, saveLabel, saveReview, teachFromReview, rescore, runAuto,
    // eval
    evalResult, runEval, history, advice, resetHistory, zoomed, setZoomed,
    // test set
    tsImages, tsLabeled, tsClasses, tsCurrent, tsSavedBoxes, tsCls, setTsCls,
    tsUpdateMode, setTsUpdateMode, ts, tsSet, tsColor, tsLabeledSet, poolCandidates,
    poolPick, setPoolPick, tsPick, setTsPick, sampleN, setSampleN,
    importToTestset, addRandomFromPool, saveTestset, removeFromTestset, goToTsImage,
  };
}

export type Session = ReturnType<typeof useSession>;
