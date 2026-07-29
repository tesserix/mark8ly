// A LOCAL mock, not the shared lib/test-support/gorhom-bottom-sheet-mock —
// that shared mock renders `children` unconditionally and its
// `present`/`dismiss` are no-ops, so every assertion in this suite would
// hold true regardless of the sheet's actual open/closed state (the
// reviewer proved all prior tests here still passed with
// `modalRef.current?.present()` deleted, with `dismiss()` deleted, and when
// rendering with `visible={false}`). A test file's own `jest.mock()` for a
// module takes precedence over jest.config.js's `moduleNameMapper`, so this
// local factory below wins for this suite only — every other suite that
// touches @gorhom/bottom-sheet still gets the shared mock.
//
// This mock tracks real presented/dismissed state, renders `children` ONLY
// while presented, and calls the `onDismiss` prop from `dismiss()` — same
// as the real library, which fires `onDismiss` once per close regardless of
// what triggered it. `dismiss()`'s early-exit guard only skips the
// state-clear + `onDismiss()` call when already dismissed — it does NOT
// skip counting the call in `dismissSpy`. Real gorhom has no such guard at
// all (see the round-2 finding this models): a redundant `dismiss()` call
// still reaches the library and parks its internal state machine at
// `MODAL_STATUS.DISMISSING`. `dismissSpy`/`presentSpy` exist so this suite
// can assert on the RAW number of imperative calls ActionSheet makes, not
// just their visible side effects — that's what round 2's "dismiss() called
// twice" bug needed and the round-1 mock (which only tracked state, not
// call count) couldn't have caught.
//
// `backdropComponent` is now actually invoked (round 1's mock left it
// unexercised — `BottomSheetBackdrop` was a no-op stub — which is exactly
// why the backdrop was deletable without a red test). `snapPointsSpy`
// captures the `snapPoints` prop on every render so the clamp/floor tests
// can assert on the exact computed height without needing to know the real
// device's `useWindowDimensions()` value.
//
// Written without JSX and without a bare `React.createElement`/`createElement`
// call anywhere in the factory — nativewind's babel transform (see
// react-native-css-interop's babel-plugin.ts) statically rewrites BOTH
// `<Foo />` JSX (this project's babel config uses the classic JSX runtime,
// so it lowers to `React.createElement` too) and any literal
// `React.createElement`/`createElement(...)` call into a reference to a
// `ReactNativeCSSInterop` helper it imports at the top of the FILE (module
// scope) — regardless of which component type is passed. Since
// jest.mock() factories are hoisted above that import, referencing it
// throws "module factory is not allowed to reference any out-of-scope
// variables" (same landmine documented in
// lib/test-support/gorhom-bottom-sheet-mock.tsx and worked around the same
// way in option-builder-sheet.test.tsx's local mock — those dodge it by
// never creating an element in the factory at all). This mock DOES need to
// create a real, queryable backdrop element (round 2 finding 2), so it
// dodges the transform differently: the plugin's detection matches only the
// literal identifier `createElement` (as `X.createElement` or a bare
// `createElement(...)` call bound to `require("react")`) — destructuring
// `createElement` under a different local name defeats that literal-name
// check while still producing a normal React element at runtime.
jest.mock("@gorhom/bottom-sheet", () => {
  const React = require("react");
  const { createElement: h } = require("react");
  const { View } = require("react-native");

  const presentSpy = jest.fn();
  const dismissSpy = jest.fn();
  const snapPointsSpy = jest.fn();

  const BottomSheetBackdrop = (props: Record<string, unknown>) =>
    h(View, { testID: "action-sheet-backdrop", ...props });

  const BottomSheetScrollView = (props: { children?: React.ReactNode; contentContainerStyle?: unknown }) =>
    h(React.Fragment, null, props.children ?? null);

  const BottomSheetModal = React.forwardRef(
    (
      props: {
        children?: React.ReactNode;
        onDismiss?: () => void;
        backdropComponent?: (props: Record<string, unknown>) => React.ReactNode;
        snapPoints?: number[];
      },
      ref: React.Ref<unknown>,
    ) => {
      const { children, onDismiss, backdropComponent, snapPoints } = props;
      snapPointsSpy(snapPoints);
      const [presented, setPresented] = React.useState(false);
      const presentedRef = React.useRef(false);
      React.useImperativeHandle(ref, () => ({
        present: () => {
          presentSpy();
          presentedRef.current = true;
          setPresented(true);
        },
        dismiss: () => {
          dismissSpy();
          if (!presentedRef.current) return;
          presentedRef.current = false;
          setPresented(false);
          onDismiss?.();
        },
      }));
      if (!presented) return null;
      const backdrop = backdropComponent ? backdropComponent({}) : null;
      return h(React.Fragment, null, backdrop, children ?? null);
    },
  );

  return {
    __esModule: true,
    BottomSheetModal,
    BottomSheetBackdrop,
    BottomSheetScrollView,
    __mockSpies: { presentSpy, dismissSpy, snapPointsSpy },
  };
});

jest.mock("@repo/mobile-shared/haptics/feedback", () => ({
  adminHaptics: {
    menuOpen: jest.fn(() => Promise.resolve()),
  },
}));

// ActionSheet now calls `useSafeAreaInsets()` (Also Fix: bottom padding
// must clear the real home-indicator inset), which throws without a
// `<SafeAreaProvider>` ancestor. react-native-safe-area-context ships an
// official jest mock for exactly this (same fix as TenantGate.test.tsx,
// security.test.tsx, new-product.test.tsx — its default insets are all 0,
// which also keeps `bottomPadding` at its `theme.spacing.xl` floor here).
jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});

import { useEffect, useRef, useState, type ComponentType, type ReactNode } from "react";
import { StyleSheet } from "react-native";
import { Text as RNText } from "react-native";
import { render, fireEvent, within } from "@testing-library/react-native";
import { BottomSheetScrollView } from "@gorhom/bottom-sheet";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import {
  ActionSheet,
  actionSheetHeight,
  type ActionSheetItem,
  type ActionSheetProps,
} from "@/components/ui/ActionSheet";
import { Eyebrow } from "@/components/ui/Eyebrow";
import { MAX_FONT_SCALE } from "@/components/ui/Text";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";
import { theme } from "@/lib/theme";

// `@gorhom/bottom-sheet`'s real types don't declare `__mockSpies` (it only
// exists on this suite's local mock above), so it's read via `require`
// rather than a named `import` (which `tsc` would reject with TS2614 — the
// real package has no such export). Same TS2786 duplicate-`@types/react`
// dodge as `ActionSheet.tsx`'s own `Backdrop`/`ScrollBody` casts for
// `BottomSheetScrollView`'s real type, which is otherwise incompatible with
// `UNSAFE_queryByType`'s signature under this project's React types.
const mockSpies = (require("@gorhom/bottom-sheet") as unknown as {
  __mockSpies: {
    presentSpy: jest.Mock;
    dismissSpy: jest.Mock;
    snapPointsSpy: jest.Mock;
  };
}).__mockSpies;
const ScrollBody = BottomSheetScrollView as unknown as ComponentType<{ children?: ReactNode }>;
const mockUseSafeAreaInsets = useSafeAreaInsets as unknown as jest.Mock;

function items(overrides: Partial<ActionSheetItem>[] = []): ActionSheetItem[] {
  const base: ActionSheetItem[] = [
    { key: "fulfil", label: "Fulfil order", onPress: jest.fn() },
    { key: "email", label: "Email label", onPress: jest.fn() },
    { key: "refund", label: "Refund", onPress: jest.fn() },
    { key: "cancel", label: "Cancel order", tone: "danger", onPress: jest.fn() },
  ];
  return base.map((item, i) => ({ ...item, ...overrides[i] }));
}

/** The single snap point the component handed the modal on its last render. */
function lastSnapPoint(): number {
  const calls = mockSpies.snapPointsSpy.mock.calls;
  return (calls[calls.length - 1][0] as number[])[0];
}

/**
 * A REAL merchant product title, not a placeholder. The only title this sheet
 * had ever been given was `Order #1042`, which cannot wrap at any text size —
 * which is exactly why the one-line height budget survived review. Products
 * (and every screen after it: customers, reviews, campaigns, tickets) passes
 * arbitrary text.
 */
const LONG_TITLE = "Insulated Stainless Steel Sport Water Bottle 1L — Bondi Edition";

function manyItems(count: number): ActionSheetItem[] {
  return Array.from({ length: count }, (_, i) => ({
    key: `item-${i}`,
    label: `Item ${i}`,
    onPress: jest.fn(),
  }));
}

/** Same contract as `ControlledActionSheet`, but exposes a numeric
 *  `reopenAt` prop so a test can simulate a real parent re-opening the sheet
 *  AFTER a full gesture-driven close round-trip has already completed —
 *  flipping `visible` straight back to `true` via `rerender` bypasses that
 *  round-trip (and the `wasVisible`/mock-spy state it exercises), which is
 *  exactly what round 2's "reopen after a gesture-close" finding needs. */
function ReopenableActionSheet({
  reopenAt,
  ...props
}: Omit<ActionSheetProps, "visible" | "onDismiss"> & { reopenAt: number }) {
  const [visible, setVisible] = useState(true);
  const lastReopenAt = useRef(reopenAt);
  useEffect(() => {
    if (reopenAt !== lastReopenAt.current) {
      lastReopenAt.current = reopenAt;
      setVisible(true);
    }
  }, [reopenAt]);
  return <ActionSheet {...props} visible={visible} onDismiss={() => setVisible(false)} />;
}

/** Mirrors the real parent contract: flip `visible` to false in response to
 *  `onDismiss`, and report every `onDismiss` call to the spy. */
function ControlledActionSheet({
  onDismissSpy,
  ...props
}: Omit<ActionSheetProps, "visible" | "onDismiss"> & { onDismissSpy: () => void }) {
  const [visible, setVisible] = useState(true);
  return (
    <ActionSheet
      {...props}
      visible={visible}
      onDismiss={() => {
        onDismissSpy();
        setVisible(false);
      }}
    />
  );
}

describe("ActionSheet", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("renders every item's label", () => {
    const { getByText } = render(
      <ActionSheet items={items()} visible onDismiss={jest.fn()} />,
    );
    expect(getByText("Fulfil order")).toBeTruthy();
    expect(getByText("Email label")).toBeTruthy();
    expect(getByText("Refund")).toBeTruthy();
    expect(getByText("Cancel order")).toBeTruthy();
  });

  it("renders an optional title", () => {
    const { getByText } = render(
      <ActionSheet title="Order #1042" items={items()} visible onDismiss={jest.fn()} />,
    );
    expect(getByText("Order #1042")).toBeTruthy();
  });

  // Rewritten: the old version only asserted the ABSENCE of a specific
  // string ("Order #1042") that was never passed in — it would pass just as
  // well against a broken implementation that always renders a hardcoded
  // title (e.g. "Menu"), since that string would never match "Order #1042"
  // either. `UNSAFE_queryByType(Eyebrow)` asserts the title block itself
  // never mounts, regardless of what text it would contain.
  it("omits the title block when none is given", () => {
    const { UNSAFE_queryByType } = render(
      <ActionSheet items={items()} visible onDismiss={jest.fn()} />,
    );
    expect(UNSAFE_queryByType(Eyebrow)).toBeNull();
  });

  it("fires the tapped item's own onPress handler, not the others", () => {
    const list = items();
    const { getByText } = render(
      <ActionSheet items={list} visible onDismiss={jest.fn()} />,
    );
    fireEvent.press(getByText("Refund"));
    expect(list[2].onPress).toHaveBeenCalledTimes(1);
    expect(list[0].onPress).not.toHaveBeenCalled();
    expect(list[1].onPress).not.toHaveBeenCalled();
    expect(list[3].onPress).not.toHaveBeenCalled();
  });

  it("dismisses after an item is tapped", () => {
    const onDismiss = jest.fn();
    const { getByText } = render(
      <ActionSheet items={items()} visible onDismiss={onDismiss} />,
    );
    fireEvent.press(getByText("Fulfil order"));
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  // IMPORTANT 1 regression test. `handlePress` used to call the `onDismiss`
  // prop directly AND `BottomSheetModal`'s own `onDismiss` was wired to that
  // same callback — so once the parent (modelled here by
  // ControlledActionSheet, per the documented prop contract) flips `visible`
  // to false in response, the component's own present/dismiss effect fires
  // a SECOND `modalRef.current?.dismiss()`, which a faithful mock (this
  // file's local one, unlike the shared no-op mock) also routes through
  // `onDismiss`. Fails on the pre-fix code (2 calls); passes on the fix
  // (1 call) because `handlePress` now only calls `modalRef.current?.dismiss()`
  // and `onDismiss` is sourced solely from the mock's own callback.
  it("fires onDismiss exactly once per row tap, through the full visible round-trip", () => {
    const onDismissSpy = jest.fn();
    const { getByText } = render(
      <ControlledActionSheet items={items()} onDismissSpy={onDismissSpy} />,
    );
    fireEvent.press(getByText("Fulfil order"));
    expect(onDismissSpy).toHaveBeenCalledTimes(1);
  });

  it("renders a danger-tone item's label in theme.colors.danger", () => {
    const { getByText } = render(
      <ActionSheet items={items()} visible onDismiss={jest.fn()} />,
    );
    const style = StyleSheet.flatten(getByText("Cancel order").props.style);
    expect(style.color).toBe(theme.colors.danger);
  });

  // Rewritten alongside the ActionSheet.tsx fix: default tone no longer
  // hardcodes `theme.colors.text` as an inline colour (which stripped the
  // preset's `text-ink` nativewind class purely so this test could read
  // `style.color`) — it now leaves `color` undefined so the design token
  // wins. `style.color` is therefore undefined here, not `theme.colors.text`.
  it("leaves a default-tone item's label on the design token, not an inline danger colour", () => {
    const { getByText } = render(
      <ActionSheet items={items()} visible onDismiss={jest.fn()} />,
    );
    const style = StyleSheet.flatten(getByText("Fulfil order").props.style) ?? {};
    expect(style.color).toBeUndefined();
    expect(style.color).not.toBe(theme.colors.danger);
  });

  // NOT covered by the row-height test below, despite inc2-task-5-report.md
  // having claimed it was. A row's `minHeight` says nothing about how many
  // line boxes its label may occupy: the 64pt box the snap-point arithmetic
  // assumes holds for a ONE-line label only, and deleting `numberOfLines`
  // leaves every other assertion in this file green while a long label at a
  // raised text size silently doubles the row.
  it("keeps every item label on ONE line, which is what the 64pt row box assumes", () => {
    const { getByText } = render(
      <ActionSheet
        items={items([{ label: "Email the shipping label to the customer" }])}
        visible
        onDismiss={jest.fn()}
      />,
    );
    expect(getByText("Email the shipping label to the customer").props.numberOfLines).toBe(1);
  });

  // Also NOT previously covered, and also claimed to be. The scroll body's
  // bottom padding must clear the real home-indicator inset, with a
  // theme.spacing.xl floor for devices that report none — the "floor at
  // handle + padding" test below exercises the SNAP POINT, not the padding
  // that is actually applied to the content.
  it("floors the content's bottom padding at the gutter and clears a real home-indicator inset", () => {
    const { UNSAFE_getByType, rerender } = render(
      <ActionSheet items={items()} visible onDismiss={jest.fn()} />,
    );
    const flat = (node: { props: { contentContainerStyle?: unknown } }) =>
      StyleSheet.flatten(node.props.contentContainerStyle) as { paddingBottom?: number };
    // Default mock insets are all 0 → the floor applies.
    expect(flat(UNSAFE_getByType(ScrollBody)).paddingBottom).toBe(theme.spacing.xl);

    mockUseSafeAreaInsets.mockReturnValue({ top: 0, bottom: 34, left: 0, right: 0 });
    rerender(<ActionSheet items={items()} visible onDismiss={jest.fn()} />);
    expect(flat(UNSAFE_getByType(ScrollBody)).paddingBottom).toBe(34);
    // Restored to the official mock's all-zero default rather than
    // mockReset(), which would strip the implementation and leave every
    // later test in this file reading `undefined` insets.
    mockUseSafeAreaInsets.mockReturnValue({ top: 0, bottom: 0, left: 0, right: 0 });
  });

  it("gives every row a real 64pt minHeight box, not hitSlop", () => {
    const { getByTestId } = render(
      <ActionSheet items={items()} visible onDismiss={jest.fn()} />,
    );
    const style = StyleSheet.flatten(getByTestId("action-sheet-item-fulfil").props.style);
    expect(style.minHeight).toBe(theme.row.minHeightSingle);
    expect(style.minHeight).toBe(64);
  });

  // Rewritten: the old assertion only checked the icon rendered SOMEWHERE
  // in the tree, which would pass even if it rendered outside its row (or
  // inside a different one). `within` scopes the query to the item's own
  // row testID and to that row also containing its label.
  it("renders each item's icon alongside its label, inside its own row", () => {
    const list = items([{ icon: <RNText testID="fulfil-icon">icon</RNText> }]);
    const { getByTestId } = render(
      <ActionSheet items={list} visible onDismiss={jest.fn()} />,
    );
    const row = within(getByTestId("action-sheet-item-fulfil"));
    expect(row.getByTestId("fulfil-icon")).toBeTruthy();
    expect(row.getByText("Fulfil order")).toBeTruthy();
  });

  it("fires adminHaptics.menuOpen once when the sheet opens", () => {
    const { rerender } = render(
      <ActionSheet items={items()} visible={false} onDismiss={jest.fn()} />,
    );
    expect(adminHaptics.menuOpen).not.toHaveBeenCalled();

    rerender(<ActionSheet items={items()} visible onDismiss={jest.fn()} />);
    expect(adminHaptics.menuOpen).toHaveBeenCalledTimes(1);
  });

  it("does not re-fire adminHaptics.menuOpen on a re-render while still visible", () => {
    const { rerender } = render(
      <ActionSheet items={items()} visible onDismiss={jest.fn()} />,
    );
    expect(adminHaptics.menuOpen).toHaveBeenCalledTimes(1);

    rerender(<ActionSheet title="Order #1042" items={items()} visible onDismiss={jest.fn()} />);
    expect(adminHaptics.menuOpen).toHaveBeenCalledTimes(1);
  });

  it("fires adminHaptics.menuOpen again on a close-then-reopen cycle", () => {
    const { rerender } = render(
      <ActionSheet items={items()} visible onDismiss={jest.fn()} />,
    );
    expect(adminHaptics.menuOpen).toHaveBeenCalledTimes(1);

    rerender(<ActionSheet items={items()} visible={false} onDismiss={jest.fn()} />);
    rerender(<ActionSheet items={items()} visible onDismiss={jest.fn()} />);
    expect(adminHaptics.menuOpen).toHaveBeenCalledTimes(2);
  });

  // IMPORTANT 2 regression tests. Under the OLD shared no-op mock, all three
  // of these held regardless of the sheet's real state — the reviewer
  // proved the whole suite stayed green with `present()` deleted, with
  // `dismiss()` deleted, and with `visible={false}`. This suite's local
  // mock (above) actually gates rendering on presented state, so each of
  // these three fails against the corresponding deleted line.
  describe("open/close state", () => {
    it("renders no items while visible is false", () => {
      const { queryByText } = render(
        <ActionSheet items={items()} visible={false} onDismiss={jest.fn()} />,
      );
      expect(queryByText("Fulfil order")).toBeNull();
    });

    it("presents the sheet on the false→true edge", () => {
      const { queryByText, rerender } = render(
        <ActionSheet items={items()} visible={false} onDismiss={jest.fn()} />,
      );
      expect(queryByText("Fulfil order")).toBeNull();

      rerender(<ActionSheet items={items()} visible onDismiss={jest.fn()} />);
      expect(queryByText("Fulfil order")).toBeTruthy();
    });

    it("dismisses the sheet on the true→false edge", () => {
      const { queryByText, rerender } = render(
        <ActionSheet items={items()} visible onDismiss={jest.fn()} />,
      );
      expect(queryByText("Fulfil order")).toBeTruthy();

      rerender(<ActionSheet items={items()} visible={false} onDismiss={jest.fn()} />);
      expect(queryByText("Fulfil order")).toBeNull();
    });
  });

  // ALSO FIX: items={[]} used to be unguarded and would present a 44pt
  // handle-only sheet with nothing in it. The guard skips presenting (and
  // the open haptic) entirely when there's nothing to show.
  it("does not present or fire the open haptic when items is empty", () => {
    const { queryByTestId } = render(
      <ActionSheet items={[]} visible onDismiss={jest.fn()} />,
    );
    expect(adminHaptics.menuOpen).not.toHaveBeenCalled();
    expect(queryByTestId(/action-sheet-item-/)).toBeNull();
  });

  // ROUND 2, FINDING 1: `dismiss()` was called a second time on an
  // already-dismissed sheet on every close. `handlePress` calls
  // `modalRef.current?.dismiss()`; gorhom fires `onDismiss`; the parent
  // (`ReopenableActionSheet`, modelling the documented contract) flips
  // `visible` to false; the present/dismiss effect's `!visible` branch then
  // fired a SECOND `dismiss()` because `wasVisible` hadn't been cleared yet.
  // `handleSheetDismissed` now clears `wasVisible.current` itself before
  // calling the `onDismiss` prop, so by the time the effect re-runs on the
  // resulting re-render, its guard already sees `wasVisible.current === false`
  // and skips the redundant call.
  it("calls the modal's dismiss() exactly once per close cycle, and a reopen after a gesture-close still presents", () => {
    const list = items();
    const { getByText, queryByText, rerender } = render(
      <ReopenableActionSheet items={list} reopenAt={0} />,
    );
    expect(mockSpies.presentSpy).toHaveBeenCalledTimes(1);

    fireEvent.press(getByText("Fulfil order"));
    expect(mockSpies.dismissSpy).toHaveBeenCalledTimes(1);
    expect(queryByText("Fulfil order")).toBeNull();

    // Reopen: the same round-trip a subsequent long-press would drive.
    rerender(<ReopenableActionSheet items={items()} reopenAt={1} />);
    expect(mockSpies.presentSpy).toHaveBeenCalledTimes(2);
    expect(getByText("Fulfil order")).toBeTruthy();
  });

  // ROUND 2, FINDING 2: the backdrop is the menu's ONLY cancel affordance
  // and is deletable without a red test under round 1's mock, which never
  // invoked the `backdropComponent` render prop. This suite's mock (above)
  // now actually calls it, so these assert on the real rendered element.
  describe("backdrop", () => {
    it("renders while the sheet is presented", () => {
      const { getByTestId } = render(
        <ActionSheet items={items()} visible onDismiss={jest.fn()} />,
      );
      expect(getByTestId("action-sheet-backdrop")).toBeTruthy();
    });

    it("is a flat opaque-ink scrim, not translucent white or a blur", () => {
      const { getByTestId } = render(
        <ActionSheet items={items()} visible onDismiss={jest.fn()} />,
      );
      const style = StyleSheet.flatten(getByTestId("action-sheet-backdrop").props.style);
      expect(style.backgroundColor).toBe(theme.colors.overlay);
      expect(style.backgroundColor).not.toBe("#FFFFFF");
      expect(style.backgroundColor).not.toMatch(/rgba\(\s*255,\s*255,\s*255/);
    });

    it('carries pressBehavior="close" so a tap outside the sheet dismisses it', () => {
      const { getByTestId } = render(
        <ActionSheet items={items()} visible onDismiss={jest.fn()} />,
      );
      expect(getByTestId("action-sheet-backdrop").props.pressBehavior).toBe("close");
    });
  });

  // ROUND 2, FINDING 3: the height clamp bounded the SHEET but not the
  // CONTENT — content sat in a plain, non-scrolling `View`, so rows past
  // the clamp were clipped and permanently unreachable. Content now sits in
  // `BottomSheetScrollView`, and the computed height carries a floor so a
  // degenerate `windowHeight` can no longer drive it to (or below) zero.
  describe("computed sheet height", () => {
    it("renders content inside BottomSheetScrollView, not a plain View, so clamped rows stay reachable", () => {
      const { UNSAFE_queryByType } = render(
        <ActionSheet items={manyItems(20)} visible onDismiss={jest.fn()} />,
      );
      expect(UNSAFE_queryByType(ScrollBody)).toBeTruthy();
    });

    it("clamps the snap-point height below the naive per-row sum for a long item list", () => {
      const count = 30;
      render(<ActionSheet items={manyItems(count)} visible onDismiss={jest.fn()} />);
      const naiveHeight = count * theme.row.minHeightSingle;
      const [computed] = mockSpies.snapPointsSpy.mock.calls[
        mockSpies.snapPointsSpy.mock.calls.length - 1
      ][0] as number[];
      expect(computed).toBeLessThan(naiveHeight);
    });

    // Floor at handle height (24) + bottom padding (theme.spacing.xl = 20,
    // since insets.bottom is 0 here) = 44. Forcing insets.top to an extreme
    // value drives `maxHeight` deeply negative regardless of the real
    // `useWindowDimensions()` value under jest, so this doesn't depend on
    // knowing the test environment's simulated device height.
    /**
     * The title's line budget.
     *
     * `snapPoints` reserves exactly `TITLE_LINES` (one) eyebrow line for the
     * title block. That held for `Order #1042` and for nothing else: `Eyebrow`
     * rendered its label with no `numberOfLines`, in the uppercased,
     * letter-spaced `eyebrow` preset, so a real product title wrapped to two
     * or three lines and the sheet came up 16–32pt short — enough to park a
     * four-item menu's last row below the fold, reachable only by scrolling
     * inside a menu that gives no hint it scrolls.
     */
    describe("title block", () => {
      it("clamps the title to one line, so arbitrary caller text cannot wrap past its budget", () => {
        const { getByText } = render(
          <ActionSheet title={LONG_TITLE} items={items()} visible onDismiss={jest.fn()} />,
        );
        expect(getByText(LONG_TITLE).props.numberOfLines).toBe(1);
      });

      it("budgets a long title at exactly the same height as a short one", () => {
        render(<ActionSheet title="Order #1042" items={items()} visible onDismiss={jest.fn()} />);
        const short = lastSnapPoint();

        render(<ActionSheet title={LONG_TITLE} items={items()} visible onDismiss={jest.fn()} />);
        expect(lastSnapPoint()).toBe(short);
      });

      // The other half of the same contract: the block must be reserved at
      // all. A budget that ignored the title would also be "the same for a
      // long title as a short one".
      it("reserves the title block in the budget, not only in the render", () => {
        render(<ActionSheet items={items()} visible onDismiss={jest.fn()} />);
        const untitled = lastSnapPoint();

        render(<ActionSheet title={LONG_TITLE} items={items()} visible onDismiss={jest.fn()} />);
        expect(lastSnapPoint()).toBeGreaterThan(untitled);
      });
    });

    it("floors the snap-point height at handle height + bottom padding when maxHeight goes negative", () => {
      mockUseSafeAreaInsets.mockReturnValueOnce({ top: 100000, bottom: 0, left: 0, right: 0 });
      render(<ActionSheet items={items()} visible onDismiss={jest.fn()} />);
      const [computed] = mockSpies.snapPointsSpy.mock.calls[
        mockSpies.snapPointsSpy.mock.calls.length - 1
      ][0] as number[];
      expect(computed).toBe(44);
      expect(computed).toBeGreaterThan(0);
    });
  });

  // ROUND 2, FINDING 4: the present/dismiss effect's empty-items guard sat
  // only on the present branch. If `items` emptied out while the sheet was
  // already open, neither branch fired, `wasVisible` stayed true, and the
  // sheet stayed presented (just shrunk to an empty body behind its
  // backdrop). The guard is now symmetric: `(!visible || !hasItems) &&
  // wasVisible.current`.
  it("dismisses the sheet if items empties out while it stays visible", () => {
    const { rerender, queryByTestId } = render(
      <ActionSheet items={items()} visible onDismiss={jest.fn()} />,
    );
    expect(queryByTestId("action-sheet-backdrop")).toBeTruthy();

    rerender(<ActionSheet items={[]} visible onDismiss={jest.fn()} />);
    expect(queryByTestId("action-sheet-backdrop")).toBeNull();
  });
});

/**
 * `disabled` items — added for Orders' long-press menu (inc2 Task 9).
 *
 * The menu shows the SAME four actions on every order (Fulfil, Email label,
 * Refund, Cancel), but only some are legal for a given order: fulfilling a
 * cancelled order is a guaranteed 409, refunding an unpaid one likewise, and
 * "Email label" needs a shipment that may not exist. The alternative —
 * varying `items` per order — changes `items.length`, which recomputes
 * `snapPoints` and resizes the sheet under the merchant's thumb (and, for
 * the lazily-fetched shipment, resizes it AFTER it has already opened).
 *
 * So the item count stays fixed and unavailable actions are disabled
 * instead. `PressableRow` already implements the whole contract (no
 * `onPress`, no press feedback, `accessibilityState.disabled` for
 * VoiceOver); this is a pass-through, not a reimplementation.
 */
describe("ActionSheet — disabled items", () => {
  it("does not fire onPress for a disabled item", () => {
    const onPress = jest.fn();
    const { getByTestId } = render(
      <ActionSheet
        items={[{ key: "fulfil", label: "Fulfil order", disabled: true, onPress }]}
        visible
        onDismiss={jest.fn()}
      />,
    );
    fireEvent.press(getByTestId("action-sheet-item-fulfil"));
    expect(onPress).not.toHaveBeenCalled();
  });

  it("still fires onPress for an enabled sibling", () => {
    const onPress = jest.fn();
    const { getByTestId } = render(
      <ActionSheet
        items={[
          { key: "fulfil", label: "Fulfil order", disabled: true, onPress: jest.fn() },
          { key: "cancel", label: "Cancel order", onPress },
        ]}
        visible
        onDismiss={jest.fn()}
      />,
    );
    fireEvent.press(getByTestId("action-sheet-item-cancel"));
    expect(onPress).toHaveBeenCalledTimes(1);
  });

  it("announces a disabled item as disabled", () => {
    const { getByTestId } = render(
      <ActionSheet
        items={[{ key: "fulfil", label: "Fulfil order", disabled: true, onPress: jest.fn() }]}
        visible
        onDismiss={jest.fn()}
      />,
    );
    expect(getByTestId("action-sheet-item-fulfil").props.accessibilityState.disabled).toBe(true);
  });

  it("keeps a disabled item in the list, so the sheet height never changes", () => {
    const { getByTestId } = render(
      <ActionSheet
        items={[
          { key: "fulfil", label: "Fulfil order", disabled: true, onPress: jest.fn() },
          { key: "cancel", label: "Cancel order", onPress: jest.fn() },
        ]}
        visible
        onDismiss={jest.fn()}
      />,
    );
    expect(getByTestId("action-sheet-item-fulfil")).toBeTruthy();
    expect(getByTestId("action-sheet-item-cancel")).toBeTruthy();
  });
});

/**
 * The height budget, as arithmetic.
 *
 * Split out of the component so it can be exercised at Dynamic Type scales
 * the jest environment does not let a test set: `useWindowDimensions` reports
 * one fixed `fontScale`, and this is precisely the part that cannot be seen
 * in a screenshot — an under-budget of one line parks the last row just below
 * the fold, where nothing about the failure is visible above it.
 *
 * The expected values are written as literals, derived in each comment, so a
 * change to the arithmetic reddens these instead of being restated by them.
 */
describe("actionSheetHeight", () => {
  // Chrome: HANDLE_HEIGHT (24) + bottomPadding (theme.spacing.xl, 20) = 44.
  // The window is a modern iPhone's, so nothing clamps at these sizes.
  const base = {
    itemCount: 4,
    hasTitle: true,
    bottomPadding: theme.spacing.xl,
    windowHeight: 852,
    topInset: 59,
  };

  // 4 × theme.row.minHeightSingle (64) = 256, title block
  // theme.spacing.lg (16) + one 16pt eyebrow line + theme.spacing.sm (8) = 40,
  // chrome 44. Total 340.
  it("fits four items and a one-line title at the default text size", () => {
    expect(actionSheetHeight({ ...base, fontScale: 1 })).toBe(340);
  });

  // Dynamic Type moves BOTH terms. A row's 17/24 label at 2× needs
  // 24×2 + theme.row.paddingV×2 (28) = 76pt, past the 64pt minHeight the
  // budget used to assume; the title line is 32 instead of 16. So
  // 4 × 76 = 304, title 16 + 32 + 8 = 56, chrome 44 → 404, i.e. 64pt more
  // than the unscaled figure. That 64pt was the whole of the on-device
  // "Archive needs an internal scroll at accessibility-large" defect.
  it("grows every row AND the title with Dynamic Type — a 64pt box does not hold a 2x label", () => {
    expect(actionSheetHeight({ ...base, fontScale: 2 })).toBe(404);
    expect(actionSheetHeight({ ...base, fontScale: 2 }) - actionSheetHeight({ ...base, fontScale: 1 })).toBe(64);
  });

  // `Text` caps every label it draws at MAX_FONT_SCALE, so budgeting against
  // the raw OS multiplier (iOS reaches ~3.1x at AX5) would reserve space for
  // text that can never be rendered.
  it("caps the budget at MAX_FONT_SCALE, because Text caps the text", () => {
    expect(actionSheetHeight({ ...base, fontScale: 3.1 })).toBe(
      actionSheetHeight({ ...base, fontScale: MAX_FONT_SCALE }),
    );
  });

  // Below 1x the row's own 64pt minHeight is what holds, not the shrunken
  // line box — the budget must not follow the text down.
  it("never budgets a row below its own minHeight at reduced text sizes", () => {
    expect(actionSheetHeight({ ...base, fontScale: 0.8 })).toBe(
      actionSheetHeight({ ...base, fontScale: 1 }),
    );
  });

  it("drops the title block entirely when there is no title", () => {
    // 340 less the 40pt title block.
    expect(actionSheetHeight({ ...base, hasTitle: false, fontScale: 1 })).toBe(300);
  });

  it("still clamps a long list under the notch, and still floors at its own chrome", () => {
    const clamped = actionSheetHeight({ ...base, itemCount: 30, fontScale: 1 });
    expect(clamped).toBe(852 - 59 - theme.spacing.xxl);
    expect(clamped).toBeLessThan(30 * theme.row.minHeightSingle);

    const degenerate = actionSheetHeight({ ...base, windowHeight: 0, fontScale: 1 });
    expect(degenerate).toBe(44);
  });
});
