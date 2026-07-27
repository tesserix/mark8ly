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
// what triggered it. `dismiss()` is idempotent (a second call while already
// dismissed is a no-op, same as the real library), which matters for
// exercising ActionSheet's own effect + handlePress interaction honestly.
//
// Written without JSX and without importing React's named exports at
// module scope — nativewind's babel transform instruments JSX/createElement
// calls with a `_ReactNativeCSSInterop` reference, and jest.mock() factories
// are hoisted above that reference's declaration, so JSX/createElement
// written directly inside a factory throws "module factory is not allowed
// to reference any out-of-scope variables" (same landmine documented in
// lib/test-support/gorhom-bottom-sheet-mock.tsx and worked around the same
// way in option-builder-sheet.test.tsx's local mock).
jest.mock("@gorhom/bottom-sheet", () => {
  const React = require("react");

  const BottomSheetModal = React.forwardRef(
    (
      props: { children?: React.ReactNode; onDismiss?: () => void },
      ref: React.Ref<unknown>,
    ) => {
      const { children, onDismiss } = props;
      const [presented, setPresented] = React.useState(false);
      const presentedRef = React.useRef(false);
      React.useImperativeHandle(ref, () => ({
        present: () => {
          presentedRef.current = true;
          setPresented(true);
        },
        dismiss: () => {
          if (!presentedRef.current) return;
          presentedRef.current = false;
          setPresented(false);
          onDismiss?.();
        },
      }));
      return presented ? (children ?? null) : null;
    },
  );

  return {
    __esModule: true,
    BottomSheetModal,
    // ActionSheet imports this for its `backdropComponent` render prop; the
    // mock above never invokes that prop, so this only needs to exist as an
    // importable symbol for module resolution + JSX inside ActionSheet.tsx
    // itself (which is NOT inside a jest.mock factory, so no landmine there).
    BottomSheetBackdrop: () => null,
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

import { useState } from "react";
import { StyleSheet } from "react-native";
import { Text as RNText } from "react-native";
import { render, fireEvent, within } from "@testing-library/react-native";
import { ActionSheet, type ActionSheetItem, type ActionSheetProps } from "@/components/ui/ActionSheet";
import { Eyebrow } from "@/components/ui/Eyebrow";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";
import { theme } from "@/lib/theme";

function items(overrides: Partial<ActionSheetItem>[] = []): ActionSheetItem[] {
  const base: ActionSheetItem[] = [
    { key: "fulfil", label: "Fulfil order", onPress: jest.fn() },
    { key: "email", label: "Email label", onPress: jest.fn() },
    { key: "refund", label: "Refund", onPress: jest.fn() },
    { key: "cancel", label: "Cancel order", tone: "danger", onPress: jest.fn() },
  ];
  return base.map((item, i) => ({ ...item, ...overrides[i] }));
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
});
