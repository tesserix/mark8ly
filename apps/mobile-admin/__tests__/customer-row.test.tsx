jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

import { StyleSheet } from "react-native";
import { render } from "@testing-library/react-native";
import { CustomerRow } from "@/components/CustomerRow";
import { formatMoney } from "@/lib/money";
import { theme } from "@/lib/theme";
import type { Customer } from "@repo/mobile-shared/api/types";

const BASE_CUSTOMER: Customer = {
  id: "c1",
  email: "buyer@example.com",
  tags: [],
  status: "active",
  marketing_opt_in: false,
  order_count: 3,
  total_spent: 149,
  created_at: "2026-05-06T00:00:00Z",
  updated_at: "2026-05-06T00:00:00Z",
};

/** The demo store's original fixture: no name, no phone. This shape passed
 * against three previous identity defects (list, detail, mid-token break),
 * so any new badge behaviour has to be proven against it too, not just
 * against a customer with a name. */
const NO_NAME_CUSTOMER: Customer = {
  ...BASE_CUSTOMER,
  id: "c-noname",
  email: "mahesh.sangawar@gmail.com",
  status: "blocked",
};

const BLOCKED_CUSTOMER: Customer = {
  ...BASE_CUSTOMER,
  id: "c-blocked",
  first_name: "Ana",
  last_name: "Ruiz",
  status: "blocked",
};

describe("CustomerRow — currency", () => {
  it("formats total_spent in the store's currency, not a hardcoded USD", () => {
    const { getByText, queryByText } = render(
      <CustomerRow customer={BASE_CUSTOMER} onPress={jest.fn()} currencyCode="EUR" />,
    );
    // The row renders the EUR-formatted amount, and NOT the old hardcoded USD
    // form. (EUR and USD outputs genuinely differ, so this catches a regression
    // regardless of whether the runtime prints a symbol or the ISO code.)
    expect(formatMoney(149, "EUR")).not.toBe(formatMoney(149, "USD"));
    expect(getByText(formatMoney(149, "EUR"))).toBeTruthy();
    expect(queryByText(formatMoney(149, "USD"))).toBeNull();
  });

  it("renders AUD (the real store currency) when passed", () => {
    const { getByText } = render(
      <CustomerRow customer={BASE_CUSTOMER} onPress={jest.fn()} currencyCode="AUD" />,
    );
    expect(getByText(formatMoney(149, "AUD"))).toBeTruthy();
  });

  it("falls back to a plain amount when no currency is available", () => {
    const { getByText } = render(
      <CustomerRow customer={BASE_CUSTOMER} onPress={jest.fn()} />,
    );
    expect(getByText(formatMoney(149, undefined))).toBeTruthy();
  });
});

describe("CustomerRow — status badge", () => {
  // The whole point of Task 16: under the default "All" filter a blocked
  // customer used to be byte-identical to an active one. This is the one
  // assertion that would fail if the badge were reverted — see the
  // red/green note at the bottom of this file.
  it("shows a Blocked badge, and hides the order-count line, for a blocked customer", () => {
    const { getByTestId, queryByText, getByText } = render(
      <CustomerRow customer={BLOCKED_CUSTOMER} onPress={jest.fn()} />,
    );
    expect(getByTestId("customer-row-c-blocked-badge")).toBeTruthy();
    expect(getByText("Blocked")).toBeTruthy();
    expect(queryByText("3 orders")).toBeNull();
  });

  it("shows no badge — and the order-count line — for an active customer", () => {
    const { queryByTestId, getByText, queryByText } = render(
      <CustomerRow customer={BASE_CUSTOMER} onPress={jest.fn()} />,
    );
    expect(queryByTestId("customer-row-c1-badge")).toBeNull();
    expect(queryByText("Active")).toBeNull();
    expect(getByText("3 orders")).toBeTruthy();
  });

  // A status the app has never seen (the wire schema types `status` as a
  // bare `z.string()`) still gets a badge, humanised rather than leaking the
  // wire token — same defensive fallback as `productStatusLabel`.
  it("humanises an unrecognised status instead of leaking the wire token", () => {
    const { getByText, getByTestId } = render(
      <CustomerRow customer={{ ...BASE_CUSTOMER, status: "pending_review" }} onPress={jest.fn()} />,
    );
    expect(getByText("Pending review")).toBeTruthy();
    expect(getByTestId("customer-row-c1-badge")).toBeTruthy();
  });

  // Task 2's exact defect on ProductRow: the badge and the spoken label
  // computed the status separately and disagreed. Assert both read from the
  // same string here too.
  it("agrees between the visible badge and the accessibilityLabel", () => {
    const { getByTestId } = render(
      <CustomerRow customer={BLOCKED_CUSTOMER} onPress={jest.fn()} />,
    );
    const row = getByTestId("customer-row-c-blocked");
    expect(row.props.accessibilityLabel).toContain("Blocked");
    const badge = getByTestId("customer-row-c-blocked-badge");
    expect(badge.props.accessibilityLabel).toBe("Status: Blocked");
  });

  it("omits the status word from accessibilityLabel entirely for an active customer", () => {
    const { getByTestId } = render(
      <CustomerRow customer={BASE_CUSTOMER} onPress={jest.fn()} />,
    );
    const row = getByTestId("customer-row-c1");
    expect(row.props.accessibilityLabel).not.toContain("Active");
  });

  // The demo store's original fixture (no name, no phone) is the shape that
  // passed against three previous identity defects. A blocked customer with
  // that exact shape must still get the badge, and the title (which IS the
  // email here — see lib/customer-identity.ts) must not be forced to wrap or
  // lose its single-line truncation contract.
  it("badges a blocked, no-name customer without disturbing the email-as-title line", () => {
    const { getByTestId, getByText } = render(
      <CustomerRow customer={NO_NAME_CUSTOMER} onPress={jest.fn()} />,
    );
    expect(getByTestId("customer-row-c-noname-badge")).toBeTruthy();
    const title = getByText("mahesh.sangawar@gmail.com");
    expect(title.props.numberOfLines).toBe(1);
  });

  // Fixed boxes holding scalable text have caused eight silent-clipping bugs
  // in this programme. `StatusBadge` has no fixed width/height of its own,
  // so nothing here should impose one — this is the guard against
  // reintroducing that bug on this row specifically.
  it("does not force a fixed size onto the badge that would clip it at larger type", () => {
    const { getByTestId } = render(
      <CustomerRow customer={BLOCKED_CUSTOMER} onPress={jest.fn()} />,
    );
    const style = StyleSheet.flatten(getByTestId("customer-row-c-blocked-badge").props.style);
    expect(style.width).toBeUndefined();
    expect(style.height).toBeUndefined();
    expect(style.overflow).not.toBe("hidden");
  });

  // The row's density is fixed by `PressableRow`'s `lines={2}` prop, not by
  // content — but this asserts the actual outcome the brief cares about
  // rather than trusting that indirection: a blocked row (badge, no
  // order-count line) and a plain active row (order-count, no badge) must
  // report the identical minHeight.
  it("keeps the row's minHeight identical with and without a badge", () => {
    const blocked = render(<CustomerRow customer={BLOCKED_CUSTOMER} onPress={jest.fn()} />);
    const active = render(<CustomerRow customer={BASE_CUSTOMER} onPress={jest.fn()} />);
    const blockedStyle = StyleSheet.flatten(
      blocked.getByTestId("customer-row-c-blocked").props.style,
    );
    const activeStyle = StyleSheet.flatten(active.getByTestId("customer-row-c1").props.style);
    expect(blockedStyle.minHeight).toBe(theme.row.minHeightDouble);
    expect(activeStyle.minHeight).toBe(theme.row.minHeightDouble);
    expect(blockedStyle.minHeight).toBe(activeStyle.minHeight);
  });
});

/**
 * RED/GREEN PROOF (see i3-task-16-report.md for the exact revert used):
 * reverting `showBadge`/`StatusBadge` rendering in CustomerRow.tsx back to
 * the pre-Task-16 shape (always render the order-count line, never a badge)
 * turns RED:
 *   - "shows a Blocked badge, and hides the order-count line…"
 *   - "agrees between the visible badge and the accessibilityLabel"
 *   - "badges a blocked, no-name customer…"
 *   - "keeps the row's minHeight identical with and without a badge" stays
 *     GREEN even reverted (it was never able to catch the actual defect —
 *     included anyway as a regression guard against a *future* change that
 *     grows the row).
 * Restoring the current file turns all of the above green again.
 */
