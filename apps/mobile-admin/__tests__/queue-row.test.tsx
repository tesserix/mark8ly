import { StyleSheet } from "react-native";
import { fireEvent, render } from "@testing-library/react-native";

jest.mock("expo-image", () => {
  const { View } = require("react-native");
  return { Image: View };
});

// Same landmine as every other lucide-consuming test in this suite (see
// __tests__/customer-row.test.tsx, __tests__/thumb.test.tsx): the
// components/ui barrel pulls in lucide-react-native's ESM build, which
// jest-expo's default transform can't parse.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

import { QueueRow } from "@/components/dashboard/QueueRow";
import { theme } from "@/lib/theme";
import type { QueueItem } from "@/lib/queue";

function baseItem(over: Partial<QueueItem> = {}): QueueItem {
  return {
    id: "o-1",
    type: "order",
    primary: "Priya Shah",
    secondary: "Order #1001",
    amount: "$42.00",
    badgeTone: "warning",
    badgeLabel: "Pending",
    onPressRoute: "/(tabs)/orders/o-1",
    ...over,
  };
}

describe("QueueRow — content", () => {
  it("renders primary, secondary, amount and badge", () => {
    const { getByText } = render(<QueueRow item={baseItem()} onPress={jest.fn()} />);
    expect(getByText("Priya Shah")).toBeTruthy();
    expect(getByText("Order #1001")).toBeTruthy();
    expect(getByText("$42.00")).toBeTruthy();
    expect(getByText("Pending")).toBeTruthy();
  });

  it("calls onPress when the row is pressed", () => {
    const onPress = jest.fn();
    const { getByTestId } = render(<QueueRow item={baseItem()} onPress={onPress} />);
    fireEvent.press(getByTestId("queue-row-o-1"));
    expect(onPress).toHaveBeenCalledTimes(1);
  });
});

describe("QueueRow — thumbnail vs monogram", () => {
  it("renders a Thumb image when imageUrl is present", () => {
    const { getByTestId, queryByTestId } = render(
      <QueueRow item={baseItem({ imageUrl: "https://cdn.example/p.jpg" })} onPress={jest.fn()} />,
    );
    expect(getByTestId("queue-row-o-1-thumb")).toBeTruthy();
    expect(queryByTestId("queue-row-o-1-monogram")).toBeNull();
  });

  it("renders a customer monogram disc for an order with no imageUrl", () => {
    const { getByTestId, getByText } = render(
      <QueueRow item={baseItem({ imageUrl: undefined, primary: "Priya Shah" })} onPress={jest.fn()} />,
    );
    expect(getByTestId("queue-row-o-1-monogram")).toBeTruthy();
    expect(getByText("P")).toBeTruthy();
  });

  it("renders a customer monogram disc for a review with no imageUrl", () => {
    const item = baseItem({
      id: "r-1",
      type: "review",
      primary: "Sam Rivera",
      imageUrl: undefined,
      badgeTone: "muted",
      badgeLabel: "New review",
    });
    const { getByTestId, getByText } = render(<QueueRow item={item} onPress={jest.fn()} />);
    expect(getByTestId("queue-row-r-1-monogram")).toBeTruthy();
    expect(getByText("S")).toBeTruthy();
  });

  it("renders a customer monogram disc for a ticket with no imageUrl", () => {
    const item = baseItem({
      id: "t-1",
      type: "ticket",
      primary: "Jamie Lee",
      imageUrl: undefined,
      badgeTone: "muted",
      badgeLabel: "Open",
    });
    const { getByTestId, getByText } = render(<QueueRow item={item} onPress={jest.fn()} />);
    expect(getByTestId("queue-row-t-1-monogram")).toBeTruthy();
    expect(getByText("J")).toBeTruthy();
  });

  it("renders the Thumb PRODUCT placeholder (not a customer monogram) for low stock — there is no customer", () => {
    const item = baseItem({
      id: "v-1",
      type: "stock",
      primary: "Linen Shirt",
      amount: undefined,
      imageUrl: undefined,
      badgeTone: "danger",
      badgeLabel: "Low stock",
    });
    const { getByTestId, queryByTestId } = render(<QueueRow item={item} onPress={jest.fn()} />);
    expect(getByTestId("queue-row-v-1-thumb")).toBeTruthy();
    expect(queryByTestId("queue-row-v-1-monogram")).toBeNull();
  });

  it("sizes the monogram disc identically to the 60pt Thumb list size", () => {
    const { getByTestId } = render(
      <QueueRow item={baseItem({ imageUrl: undefined })} onPress={jest.fn()} />,
    );
    const style = StyleSheet.flatten(getByTestId("queue-row-o-1-monogram").props.style);
    expect(style.width).toBe(theme.thumb.list);
    expect(style.height).toBe(theme.thumb.list);
  });
});

describe("QueueRow — 'See all' row", () => {
  const seeAllItem: QueueItem = {
    id: "see-all-order",
    type: "order",
    primary: "See all 9 pending orders",
    secondary: "",
    onPressRoute: "/(tabs)/orders",
  };

  it("renders only the primary text — no thumb, no monogram, no badge, no amount", () => {
    const { getByText, queryByTestId, queryByText } = render(
      <QueueRow item={seeAllItem} onPress={jest.fn()} />,
    );
    expect(getByText("See all 9 pending orders")).toBeTruthy();
    expect(queryByTestId("queue-row-see-all-order-thumb")).toBeNull();
    expect(queryByTestId("queue-row-see-all-order-monogram")).toBeNull();
    expect(queryByText("Pending")).toBeNull();
  });

  it("calls onPress when pressed", () => {
    const onPress = jest.fn();
    const { getByTestId } = render(<QueueRow item={seeAllItem} onPress={onPress} />);
    fireEvent.press(getByTestId("queue-row-see-all-order"));
    expect(onPress).toHaveBeenCalledTimes(1);
  });
});

describe("QueueRow — native row density", () => {
  it("renders a two-line item row at the 88pt double-line height", () => {
    const { getByTestId } = render(<QueueRow item={baseItem()} onPress={jest.fn()} />);
    const style = StyleSheet.flatten(getByTestId("queue-row-o-1").props.style);
    expect(style.minHeight).toBe(theme.row.minHeightDouble);
    expect(style.paddingHorizontal).toBe(theme.row.paddingH);
  });

  it("renders a 'See all' row at the 64pt single-line height", () => {
    const item: QueueItem = {
      id: "see-all-stock",
      type: "stock",
      primary: "See all low stock items",
      secondary: "",
      onPressRoute: "/(tabs)/products",
    };
    const { getByTestId } = render(<QueueRow item={item} onPress={jest.fn()} />);
    const style = StyleSheet.flatten(getByTestId("queue-row-see-all-stock").props.style);
    expect(style.minHeight).toBe(theme.row.minHeightSingle);
  });

  it("does not apply an opacity press style (the NativeWind function-style landmine class)", () => {
    const { getByTestId } = render(<QueueRow item={baseItem()} onPress={jest.fn()} />);
    const style = StyleSheet.flatten(getByTestId("queue-row-o-1").props.style);
    expect(style.opacity).toBeUndefined();
  });
});
