// The Orders list row, rebuilt to the inc2 Task 9 spec: 88pt, order number +
// customer name at 17/600 with the status badge right, and a serif tabular
// total with the relative time on the second line.
//
// `OrderRow` was EVOLVED rather than rebuilt inside the screen: it has
// exactly one call site (app/(tabs)/orders/index.tsx) but the screen file
// already carries the header, chips, scroll-reveal search, swipe wiring and
// four sheets. Keeping the row a separate ~90-line file keeps both under the
// project's file-size guidance and lets the row's layout be pinned here
// without rendering the whole screen and mocking six hooks.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

import { StyleSheet } from "react-native";
import { render, fireEvent } from "@testing-library/react-native";
import { OrderRow } from "@/components/OrderRow";
import { theme } from "@/lib/theme";
import type { Order } from "@repo/mobile-shared/api/types";

function order(over: Partial<Order> = {}): Order {
  return {
    id: "o1",
    order_number: "1042",
    customer_email: "ana@example.com",
    customer_name: "Ana Ruiz",
    status: "pending",
    payment_status: "pending",
    fulfillment_status: "unfulfilled",
    grand_total: 189,
    refunded_amount: 0,
    currency_code: "AUD",
    placed_at: new Date(Date.now() - 12 * 60_000).toISOString(),
    ...over,
  } as unknown as Order;
}

function renderRow(over: Partial<Order> = {}, props: Record<string, unknown> = {}) {
  const onPress = jest.fn();
  const onLongPress = jest.fn();
  return {
    onPress,
    onLongPress,
    ...render(
      <OrderRow order={order(over)} onPress={onPress} onLongPress={onLongPress} {...props} />,
    ),
  };
}

describe("OrderRow — density", () => {
  it("renders at the 88pt two-line native height on the 20pt gutter", () => {
    const { getByTestId } = renderRow();
    const style = StyleSheet.flatten(getByTestId("order-row-o1").props.style);
    expect(style.minHeight).toBe(theme.row.minHeightDouble);
    expect(style.paddingHorizontal).toBe(theme.row.paddingH);
  });

  it("does not apply a whole-row opacity press fade", () => {
    const { getByTestId } = renderRow();
    const style = StyleSheet.flatten(getByTestId("order-row-o1").props.style);
    expect(style.opacity).toBeUndefined();
  });
});

describe("OrderRow — first line", () => {
  it("sets the order number and customer name as one 17/600 line", () => {
    const { getByText } = renderRow();
    const line = getByText("#1042 · Ana Ruiz");
    expect(line.props.className).toContain("font-sans-semibold");
    expect(line.props.className).toContain("text-body");
  });

  it("falls back to the customer email when no name is on the order", () => {
    const { getByText } = renderRow({ customer_name: undefined });
    expect(getByText("#1042 · ana@example.com")).toBeTruthy();
  });

  // The brief says "status badge right" — ONE badge. The row used to render
  // the order status AND the payment status side by side, which put two
  // chips on an 88pt row whose right column also carries the total.
  it("shows exactly one status badge, the order status", () => {
    const { getAllByTestId, getByLabelText } = renderRow();
    expect(getAllByTestId("order-row-o1-badge")).toHaveLength(1);
    expect(getByLabelText("Status: Pending")).toBeTruthy();
  });
});

describe("OrderRow — second line", () => {
  it("sets the total in serif at h3, with tabular figures", () => {
    const { getByTestId } = renderRow();
    const total = getByTestId("order-row-o1-total");
    expect(total.props.className).toContain("font-serif");
    expect(total.props.className).toContain("text-h3");
    expect(StyleSheet.flatten(total.props.style).fontVariant).toContain("tabular-nums");
  });

  it("formats the total in the order's own currency", () => {
    const { getByTestId } = renderRow();
    expect(getByTestId("order-row-o1-total").props.children).toBe("$189.00");
  });

  it("shows how long ago the order was placed", () => {
    const { getByText } = renderRow();
    expect(getByText("12m ago")).toBeTruthy();
  });
});

describe("OrderRow — interaction", () => {
  it("reports a tap with the order", () => {
    const { getByTestId, onPress } = renderRow();
    fireEvent.press(getByTestId("order-row-o1"));
    expect(onPress).toHaveBeenCalledWith(expect.objectContaining({ id: "o1" }));
  });

  // The long-press is what opens the ActionSheet on the screen above.
  it("reports a long press with the order", () => {
    const { getByTestId, onLongPress } = renderRow();
    fireEvent(getByTestId("order-row-o1"), "longPress");
    expect(onLongPress).toHaveBeenCalledWith(expect.objectContaining({ id: "o1" }));
  });

  it("announces the whole row to a screen reader", () => {
    const { getByTestId } = renderRow();
    const label = getByTestId("order-row-o1").props.accessibilityLabel as string;
    expect(label).toContain("1042");
    expect(label).toContain("Ana Ruiz");
    expect(label).toContain("$189.00");
    expect(label).toContain("Pending");
  });

  // A row with no long-press handler must still be tappable — the prop is
  // optional so the row stays reusable outside the gesture-triage screen.
  it("works without a long-press handler", () => {
    const onPress = jest.fn();
    const { getByTestId } = render(<OrderRow order={order()} onPress={onPress} />);
    fireEvent.press(getByTestId("order-row-o1"));
    expect(onPress).toHaveBeenCalledTimes(1);
  });
});
