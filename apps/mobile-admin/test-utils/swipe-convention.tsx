import type { render } from "@testing-library/react-native";

/**
 * The app-wide swipe convention, as a reusable assertion.
 *
 * WHICH SIDE and WHICH COLOUR, not just "the action exists".
 *
 * `SwipeRow` gives leading and trailing buttons the SAME testID pattern
 * (`${testID}-action-${key}`), so every test that reaches an action by its id
 * passes identically whether Cancel sits on the leading edge or the trailing
 * one: swapping the two props left both the Orders and the Dashboard suites
 * fully green while putting the destructive action under the constructive
 * gesture. Tone had no assertion anywhere either, so painting Cancel moss was
 * equally free.
 *
 * In an app with no undo, the side/colour pairing IS the safety property — a
 * merchant's thumb learns "right is safe, left is not" across every list, and
 * one screen that inverts it is worse than one that has no gesture at all.
 * Increment 3 puts swipe actions on several more screens, so the net is
 * hoisted out of the two screens that discovered the gap and into one place
 * every new screen's suite calls in a single line.
 *
 * NOT covered here, deliberately: each screen's POSITIVE per-action
 * assertions (`approve` is leading + accent, `cancel` is trailing + danger,
 * a ticket's `close` is trailing + neutral, and the paint assertions that
 * catch a tone→colour drift). Those are screen-specific facts, not an
 * invariant, and they must stay in the screen's own suite.
 *
 * This file lives in `test-utils/`, NOT `__tests__/` — jest's default
 * `testMatch` treats every file under `__tests__/` as a suite, and a helper
 * with no `it()` in it fails the run with "Your test suite must contain at
 * least one test".
 */
export const CONSTRUCTIVE_TONE = "accent";
export const DESTRUCTIVE_TONE = "danger";

export type Root = ReturnType<typeof render>["UNSAFE_root"];

export interface RowActions {
  leadingActions?: { key: string; tone: string; autoFireOnFullSwipe?: boolean }[];
  trailingActions?: { key: string; tone: string; autoFireOnFullSwipe?: boolean }[];
}

/** Every mounted `SwipeRow` element, so its props can be read directly. */
export function swipeRows(root: Root) {
  return root.findAll(
    (n) => typeof n.type !== "string" && (n.type as { name?: string }).name === "SwipeRow",
  );
}

export function swipeRow(root: Root, testID: string) {
  return swipeRows(root).find((r) => r.props.testID === testID);
}

/** Asserts the app-wide side/tone invariant over EVERY mounted SwipeRow. */
export function assertSwipeConvention(root: Root): void {
  const rows = swipeRows(root);
  // Fails loudly if the finder found nothing — `SwipeRow` must stay a NAMED
  // function export or every one of these assertions vacuously passes.
  expect(rows.length).toBeGreaterThan(0);
  for (const row of rows) {
    const { leadingActions = [], trailingActions = [] } = row.props as RowActions;
    for (const action of leadingActions) expect(action.tone).not.toBe(DESTRUCTIVE_TONE);
    for (const action of trailingActions) expect(action.tone).not.toBe(CONSTRUCTIVE_TONE);
  }
}

/**
 * Asserts no action on any row opts into full-swipe auto-fire. This app has
 * no undo, so nothing may fire from the drag itself — a revealed action is
 * always tapped.
 */
export function assertNoAutoFire(root: Root): void {
  const rows = swipeRows(root);
  expect(rows.length).toBeGreaterThan(0);
  for (const row of rows) {
    const { leadingActions = [], trailingActions = [] } = row.props as RowActions;
    for (const a of [...leadingActions, ...trailingActions]) {
      expect(a.autoFireOnFullSwipe).toBeFalsy();
    }
  }
}
