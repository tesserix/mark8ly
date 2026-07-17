jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

import { render } from "@testing-library/react-native";
import { CustomerRow } from "@/components/CustomerRow";
import { formatMoney } from "@/lib/money";
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
