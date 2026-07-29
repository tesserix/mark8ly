import { useCallback, useMemo, useRef, useState } from "react";
import { describeActionFailure } from "./action-failure-message";

/**
 * The most recent failed direct mutation on one screen.
 *
 * `key` is a monotonic counter, NOT a hash of the copy. Two failures of the
 * same action produce byte-identical `title`/`detail`, and a surface that
 * deduped on the text would silently swallow the second one — including its
 * screen-reader announcement, which is the whole point for a merchant who
 * cannot see the strip.
 */
export interface ActionFailure {
  key: number;
  title: string;
  detail: string;
}

export interface ActionFailureState {
  failure: ActionFailure | null;
  /**
   * Records a failure, REPLACING any previous one.
   *
   * Replace rather than append, deliberately. Triage is a queue: a merchant
   * works down a list firing actions and does not wait for each to settle, so
   * a stack of banners would grow past the screen and bury the newest —
   * the most relevant — message under stale ones. One strip, always the
   * latest, is readable at any rate of failure.
   *
   * @param action A lower-case verb phrase, read after "Couldn't " — e.g.
   *   `"archive this product"`.
   */
  reportFailure: (error: unknown, action: string) => void;
  /** Retires the notice: dismissal, or a later success on the same screen. */
  clearFailure: () => void;
}

/**
 * Screen-local state behind the mutation-failure strip.
 *
 * Screen-LOCAL, not a module store or a context: the notice belongs to the
 * list the merchant is looking at, and a global one would follow them onto
 * the next tab and outlive the row it describes. It also keeps every screen
 * test able to render the screen and assert on the message without a
 * provider.
 *
 * Nothing here expires on a timer. An auto-dismissing message is a WCAG 2.1
 * SC 2.2.1 problem (a time limit on reading content) and is exactly wrong for
 * the person this surface is for — a merchant mid-triage who looks back at
 * the screen a few seconds later. It clears on an explicit dismiss, on the
 * next success, or by being replaced.
 */
export function useActionFailure(): ActionFailureState {
  const [failure, setFailure] = useState<ActionFailure | null>(null);
  // A ref, not state: it only ever feeds the next value and is never rendered
  // on its own, so bumping it must not schedule a render of its own.
  const nextKey = useRef(0);

  const reportFailure = useCallback((error: unknown, action: string) => {
    nextKey.current += 1;
    const { title, detail } = describeActionFailure(error, action);
    setFailure({ key: nextKey.current, title, detail });
  }, []);

  // Returns `prev` untouched when there is nothing to clear, so the success
  // path of every settled mutation doesn't schedule a render for nothing.
  const clearFailure = useCallback(() => {
    setFailure((prev) => (prev === null ? prev : null));
  }, []);

  return useMemo(
    () => ({ failure, reportFailure, clearFailure }),
    [failure, reportFailure, clearFailure],
  );
}
