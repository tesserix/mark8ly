import { describe, expect, it } from "vitest";
import { render, screen, within } from "@testing-library/react";

import { MySalesToday } from "./MySalesToday";

const sale = (id: string, n: string, at: string, total: string, method?: string) => ({
  id, order_number: n, grand_total: total, currency_code: "AUD", payment_method: method, placed_at: at,
});

describe("MySalesToday", () => {
  it("lists order number, method and total, newest first", () => {
    render(
      <MySalesToday
        currencyCode="AUD"
        sales={[
          sale("a", "M-1", "2026-11-15T01:00:00Z", "110.00", "card_external"),
          sale("b", "M-2", "2026-11-15T03:00:00Z", "28.00", "cash"),
        ]}
      />,
    );
    const rows = within(screen.getByRole("region", { name: "Your sales today" })).getAllByRole("listitem");
    expect(rows[0]).toHaveTextContent("M-2");
    expect(rows[0]).toHaveTextContent("Cash");
    expect(rows[0]).toHaveTextContent("A$28.00");
    expect(rows[1]).toHaveTextContent("Card (EFTPOS)");
  });

  it("says when there are no sales yet", () => {
    render(<MySalesToday currencyCode="AUD" sales={[]} />);
    expect(screen.getByText("No sales yet today.")).toBeInTheDocument();
  });

  it("says when the list couldn't be loaded", () => {
    render(<MySalesToday currencyCode="AUD" sales={null} />);
    expect(screen.getByText(/couldn.t load your sales/i)).toBeInTheDocument();
  });
});
