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

function baseItem(over: Partial<Extract<QueueItem, { kind: "item" }>> = {}): QueueItem {
  return {
    kind: "item",
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

  it("sizes the monogram tile identically to the 60pt Thumb list size", () => {
    const { getByTestId } = render(
      <QueueRow item={baseItem({ imageUrl: undefined })} onPress={jest.fn()} />,
    );
    const style = StyleSheet.flatten(getByTestId("queue-row-o-1-monogram").props.style);
    expect(style.width).toBe(theme.thumb.list);
    expect(style.height).toBe(theme.thumb.list);
  });

  // One leading-art SHAPE per list. The monogram was a full circle
  // (`thumb.list / 2`) while Thumb is a rounded square, so every screenshot
  // showed circles beside a square — and an order that happened to carry an
  // `image_url` rendered a square between circles. Fixed on the Monogram;
  // Thumb is the shape the rest of the app's lists already use, so it is NOT
  // the thing that moves.
  it("gives the monogram Thumb's rounded-rect radius, not a circle", () => {
    const { getByTestId } = render(
      <QueueRow item={baseItem({ imageUrl: undefined })} onPress={jest.fn()} />,
    );
    const monogram = StyleSheet.flatten(
      getByTestId("queue-row-o-1-monogram").props.style,
    );
    expect(monogram.borderRadius).toBe(theme.radii.md);
    expect(monogram.borderRadius).not.toBe(theme.thumb.list / 2);
  });

  it("renders the monogram and the Thumb at the same radius inside one list", () => {
    const withPhoto = render(
      <QueueRow
        item={baseItem({ id: "o-2", imageUrl: "https://cdn.example/p.jpg" })}
        onPress={jest.fn()}
      />,
    );
    const withoutPhoto = render(
      <QueueRow item={baseItem({ imageUrl: undefined })} onPress={jest.fn()} />,
    );
    const thumbRadius = StyleSheet.flatten(
      withPhoto.getByTestId("queue-row-o-2-thumb").props.style,
    ).borderRadius;
    const monogramRadius = StyleSheet.flatten(
      withoutPhoto.getByTestId("queue-row-o-1-monogram").props.style,
    ).borderRadius;
    expect(monogramRadius).toBe(thumbRadius);
  });

  // Guards the fix for the monogram vanishing under PressableRow's iOS
  // pressed state: PressableRow repaints the row to `theme.colors.sink`,
  // which is also the monogram's own fill, so a fill-only disc has zero
  // edge contrast while held (confirmed on-device, see
  // inc2-task-7-report.md "Fix round 1"). This can't catch the visual bug
  // itself (RNTL doesn't render pressed state or actual pixels — only a
  // device screenshot does), but it does lock in the two properties that
  // make the on-device fix possible: a border exists, and it is NOT the
  // same token as the fill it would otherwise vanish against.
  it("rings the monogram disc with a border distinct from its own fill (stays visible when the row's pressed background matches the fill)", () => {
    const { getByTestId } = render(
      <QueueRow item={baseItem({ imageUrl: undefined })} onPress={jest.fn()} />,
    );
    const style = StyleSheet.flatten(getByTestId("queue-row-o-1-monogram").props.style);
    expect(style.borderWidth).toBeGreaterThan(0);
    expect(style.borderColor).toBeDefined();
    expect(style.borderColor).not.toBe(style.backgroundColor);
    // The fill itself must still be `sink` — PressableRow's pressed
    // background token — per the module's own documented rationale; this
    // fix adds a ring, it does not change the fill.
    expect(style.backgroundColor).toBe(theme.colors.sink);
  });
});

describe("QueueRow — 'See all' row", () => {
  const seeAllItem: QueueItem = {
    kind: "seeAll",
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

  // One accent per view, and the Dashboard's is already spent on the revenue
  // chart and the Approve swipe. A moss "See all" link was a third — found on
  // device, and reverting the fix to `color="accent"` left all 22 dashboard
  // tests green, so the rule had zero coverage. The chevron already carries
  // the affordance; the colour does not need to.
  //
  // Asserted on the utility class, not a flattened style: NativeWind compiles
  // classNames natively and does not resolve them to RN style objects under
  // jest, so a `style.color` assertion would pass vacuously for both.
  it("renders the 'See all' label in ink, never in the moss accent", () => {
    const { getByText } = render(<QueueRow item={seeAllItem} onPress={jest.fn()} />);
    const className = getByText("See all 9 pending orders").props.className as string;
    expect(className).toMatch(/\btext-ink\b/);
    expect(className).not.toMatch(/\btext-moss\b/);
  });
});

describe("QueueRow — row kind discriminant", () => {
  // Prior to this fix, `QueueRow` used `item.badgeTone === undefined` as
  // the row-kind discriminator, and `badgeTone` was optional even on real
  // rows — so a fully-populated order item that merely forgot to set
  // `badgeTone` type-checked and silently rendered as a single-line "See
  // all" link, dropping its amount/photo/badge with no error. `QueueItem`
  // is now a discriminated union keyed on `kind`, with `badgeTone` REQUIRED
  // on the `"item"` variant — so that state can no longer be constructed at
  // all (see the `@ts-expect-error` compile-time guard in queue.test.ts).
  // This test locks in the runtime half: `kind`, not `badgeTone`, drives
  // the layout choice.
  it("renders the full item layout for kind:'item' even though badgeTone alone used to be the discriminator", () => {
    const item = baseItem({ amount: "$142.00", imageUrl: "https://cdn.example/p.jpg" });
    const { getByText, getByTestId, queryByText } = render(
      <QueueRow item={item} onPress={jest.fn()} />,
    );
    expect(getByText("Priya Shah")).toBeTruthy();
    expect(getByText("$142.00")).toBeTruthy();
    expect(getByText("Pending")).toBeTruthy();
    expect(getByTestId("queue-row-o-1-thumb")).toBeTruthy();
    expect(queryByText("See all")).toBeNull();
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
      kind: "seeAll",
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
