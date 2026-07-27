// `@gorhom/bottom-sheet` is globally mapped in jest.config.js to
// lib/test-support/gorhom-bottom-sheet-mock.tsx — no local jest.mock() here.
// (A local factory that itself `require()`s the same globally-mapped module
// recurses infinitely, since Jest resolves the factory's own module id
// through the same moduleNameMapper entry it's trying to mock.)

jest.mock("@repo/mobile-shared/haptics/feedback", () => ({
  adminHaptics: {
    menuOpen: jest.fn(() => Promise.resolve()),
  },
}));

import { StyleSheet } from "react-native";
import { Text as RNText } from "react-native";
import { render, fireEvent } from "@testing-library/react-native";
import { ActionSheet, type ActionSheetItem } from "@/components/ui/ActionSheet";
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

  it("omits the title block when none is given", () => {
    const { queryByText } = render(
      <ActionSheet items={items()} visible onDismiss={jest.fn()} />,
    );
    expect(queryByText("Order #1042")).toBeNull();
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

  it("renders a danger-tone item's label in theme.colors.danger", () => {
    const { getByText } = render(
      <ActionSheet items={items()} visible onDismiss={jest.fn()} />,
    );
    const style = StyleSheet.flatten(getByText("Cancel order").props.style);
    expect(style.color).toBe(theme.colors.danger);
  });

  it("renders a default-tone item's label in theme.colors.text, not danger", () => {
    const { getByText } = render(
      <ActionSheet items={items()} visible onDismiss={jest.fn()} />,
    );
    const style = StyleSheet.flatten(getByText("Fulfil order").props.style);
    expect(style.color).toBe(theme.colors.text);
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

  it("renders each item's icon alongside its label", () => {
    const list = items([{ icon: <RNText testID="fulfil-icon">icon</RNText> }]);
    const { getByTestId } = render(
      <ActionSheet items={list} visible onDismiss={jest.fn()} />,
    );
    expect(getByTestId("fulfil-icon")).toBeTruthy();
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
});
