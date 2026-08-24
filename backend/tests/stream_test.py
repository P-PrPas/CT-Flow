"""The one thing that has to stay true about the sidecar's NDJSON streams:
a pass is produced on a single dedicated thread, start to finish.

    python -m backend.tests.stream_test

inference/vpe.py guards each checkpoint with a threading.RLock held across a
whole batch (see armed()), because arm() and set_prompts() write state that
predict() reads straight back. A threading lock belongs to the thread that took
it. Hand a *sync* generator to StreamingResponse and Starlette pulls it through
iterate_in_threadpool, which runs every next() on whichever anyio worker is
free -- so the release lands on a thread that never acquired it. That raises
RuntimeError, and leaves the lock held at count=1 for the life of the process:
every later request touching that checkpoint blocks until a restart.

Note how this is checked. Racing two passes and waiting for the RuntimeError
does reproduce it, but only about a third of the time -- anyio hands back the
most recently idle worker, so a pass often keeps its thread by luck. A test
that catches a regression on one run in three is not a test. What is asserted
instead is the property that makes the race impossible: the lines come from one
thread, and it is _ndjson's own, not a pool worker's. That fails every time on
a sync-generator implementation, immediately, with no timing involved.

The concurrent pair below is still run, because the queue plumbing has to work
with two passes in flight -- it just is not what does the detecting.

No model is loaded here, but importing the sidecar pulls torch in, so this runs
alongside bank_test.py rather than in the no-torch CI job.
"""
import asyncio
import threading
from contextlib import contextmanager

from ..inference.service import _ndjson

LINES = 6
# _ndjson names the thread it produces on. Asserting the name (rather than just
# "some single thread") is what makes this deterministic: Starlette's workers
# are called "AnyIO worker thread", so a sync-generator _ndjson fails here on
# every run instead of on the unlucky ones.
PRODUCER_THREAD = "vpe-stream"


@contextmanager
def _armed(lock, seen: set):
    """The shape of inference/vpe.py::armed, minus the model."""
    lock.acquire()
    seen.add(threading.current_thread().name)
    try:
        yield
    finally:
        seen.add(threading.current_thread().name)
        lock.release()


def _pass(lock, seen: set):
    """The shape of service.py::predict_stream's lines()."""
    with _armed(lock, seen):
        for i in range(LINES):
            # Blocking work between yields, like a real predict_one(): a
            # generator that never gives the loop a chance to reschedule it
            # would not exercise anything.
            threading.Event().wait(0.01)
            seen.add(threading.current_thread().name)
            yield {"image": f"/img/{i}.jpg", "boxes": []}
    yield {"done": True}


async def _drain(lock, seen: set) -> list[str]:
    return [chunk async for chunk in _ndjson(_pass(lock, seen)).body_iterator]


async def _two_at_once():
    # Two locks, not one: two projects on two checkpoints, which is the case
    # that genuinely runs in parallel rather than queueing on the same lock.
    state = [(threading.RLock(), set()) for _ in "AB"]
    bodies = await asyncio.gather(*(_drain(lock, seen) for lock, seen in state))
    return [(body, lock, seen) for body, (lock, seen) in zip(bodies, state)]


def main():
    for name, (chunks, lock, seen) in zip("AB", asyncio.run(_two_at_once())):
        # The check that does the work. Anything other than exactly one
        # dedicated producer thread means the lock can be released by a thread
        # that never took it.
        assert seen == {PRODUCER_THREAD}, (
            f"{name}: the pass ran on {sorted(seen)}, want only [{PRODUCER_THREAD!r}] -- "
            "a sync generator handed to StreamingResponse is iterated across pool "
            "threads, which breaks inference/vpe.py's per-checkpoint RLock")

        # An error line rather than an exception: _ndjson reports whatever the
        # producer raised in-band, so a cross-thread release arrives here as a
        # line, not a traceback.
        assert not any('"error"' in c for c in chunks), f"{name}: {chunks[-1]!r}"
        assert len(chunks) == LINES + 1, f"{name}: {len(chunks)} lines, want {LINES + 1}"
        assert chunks[-1].strip() == '{"done": true}', f"{name}: no terminator: {chunks[-1]!r}"

        # The real damage is not the failed pass, it is the lock nobody can ever
        # take again. acquire(blocking=False) is the only way to ask.
        assert lock.acquire(blocking=False), f"{name}: lock still held after the pass"
        lock.release()

    print(f"stream_test OK -- both passes produced on {PRODUCER_THREAD!r} and released their locks")


if __name__ == "__main__":
    main()
