import { useCallback, useMemo, useState } from "react";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";

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
   * releases that row's guard by id and reports the outcome in the hand.
   * There is nothing to roll back — no local state ever claimed the row
   * changed.
   */
  settleCallbacks: (id: string) => { onSuccess: () => void; onError: () => void };
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

  const settleCallbacks = useCallback(
    (id: string) => ({
      onSuccess: () => {
        clearBusy(id);
        void adminHaptics.actionSucceeded();
      },
      onError: () => {
        clearBusy(id);
        void adminHaptics.actionFailed();
      },
    }),
    [clearBusy],
  );

  return useMemo(
    () => ({ isBusy, markBusy, clearBusy, settleCallbacks }),
    [isBusy, markBusy, clearBusy, settleCallbacks],
  );
}
