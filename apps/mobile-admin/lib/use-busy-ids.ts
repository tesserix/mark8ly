import { useCallback, useMemo, useState } from "react";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";
import { useActionFailure, type ActionFailure } from "./use-action-failure";

/**
 * Per-row, per-request gesture guard for a list a merchant works THROUGH.
 *
 * Extracted from Orders, where it was written inline, so every list screen in
 * increment 3 inherits the SET semantics rather than each one re-deriving
 * them (and re-deriving the single-slot bug the set exists to fix — see
 * `markBusy`).
 *
 * This is a guard on the CONTROL, keyed purely on the mutation lifecycle. It
 * is never evidence about the DATA: "the request stopped" is a fine reason to
 * re-arm a gesture and NOT a claim that fresh list contents arrived. Two
 * shipped Dashboard bugs came from conflating the two; don't reintroduce the
 * conflation by reading this set as "the row changed".
 */
export interface BusyIds {
  /** True while THIS row's own request is open. */
  isBusy: (id: string) => boolean;
  markBusy: (id: string) => void;
  clearBusy: (id: string) => void;
  /**
   * react-query callbacks for a direct (non-sheet) mutation on one row:
   * releases that row's guard by id, reports the outcome in the hand, and —
   * when given an action label — puts a merchant-readable reason on screen.
   * There is nothing to roll back — no local state ever claimed the row
   * changed.
   *
   * @param action A lower-case verb phrase naming what the merchant tried,
   *   read after "Couldn't " — e.g. `"archive this product"`. OPTIONAL only
   *   so that this stayed additive when it was introduced: every call site
   *   passed just an id, and a changed default would have rippled across
   *   every screen in the increment. Omitting it means the failure is
   *   reported in the hand ALONE, which is the silence this exists to fix —
   *   so pass it.
   */
  settleCallbacks: (
    id: string,
    action?: string,
  ) => {
    onSuccess: () => void;
    /**
     * The parameter is the mutation's error. react-query already passes it
     * (`onError(error, variables, context)`), so widening the signature is
     * invisible to every existing call site — they simply hand the whole
     * object to `mutate` as before.
     */
    onError: (error?: unknown) => void;
  };
  /**
   * The most recent failed direct mutation on this screen, or null.
   * Render it with `<ActionFailureNotice />`.
   */
  failure: ActionFailure | null;
  dismissFailure: () => void;
}

export function useBusyIds(): BusyIds {
  // A SET, not one slot: triage is a queue and a merchant fires the next row
  // long before the previous one's request comes back. With a single slot,
  // marking B overwrote A's guard — and A's `onSuccess` then cleared it
  // outright, re-arming B while B's own request was still open. Replaced
  // immutably (never `.add`/`.delete` on the live value) so React sees a new
  // identity and memoised renderItems actually re-run.
  const [ids, setIds] = useState<ReadonlySet<string>>(() => new Set());

  const markBusy = useCallback((id: string) => {
    setIds((prev) => (prev.has(id) ? prev : new Set(prev).add(id)));
  }, []);

  const clearBusy = useCallback((id: string) => {
    setIds((prev) => {
      if (!prev.has(id)) return prev;
      const next = new Set(prev);
      next.delete(id);
      return next;
    });
  }, []);

  const isBusy = useCallback((id: string) => ids.has(id), [ids]);

  // The screen-local notice this hook feeds. Composed rather than inlined so
  // the Dashboard — which builds its own optimistic-hide callbacks and does
  // NOT use this hook — can reach the same surface directly.
  const { failure, reportFailure, clearFailure } = useActionFailure();

  const settleCallbacks = useCallback(
    (id: string, action?: string) => ({
      onSuccess: () => {
        clearBusy(id);
        // A success retires the last failure: a notice about the attempt
        // before a successful retry is simply untrue, and it would sit there
        // contradicting the row the merchant just watched change.
        clearFailure();
        void adminHaptics.actionSucceeded();
      },
      onError: (error?: unknown) => {
        clearBusy(id);
        if (action !== undefined) reportFailure(error, action);
        void adminHaptics.actionFailed();
      },
    }),
    // All three are stable `useCallback`s, so `settleCallbacks` keeps its
    // identity across a failure. Products/Orders/Customers memoise their row
    // actions on THIS function specifically — if it churned, every row's
    // actions and every memoised `renderItem` would be rebuilt each time a
    // mutation failed.
    [clearBusy, reportFailure, clearFailure],
  );

  return useMemo(
    () => ({
      isBusy,
      markBusy,
      clearBusy,
      settleCallbacks,
      failure,
      dismissFailure: clearFailure,
    }),
    [isBusy, markBusy, clearBusy, settleCallbacks, failure, clearFailure],
  );
}
