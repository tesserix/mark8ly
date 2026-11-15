import { describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

const setCashierDiscountsAction = vi.fn();
vi.mock("@/app/(admin)/settings/team/actions", () => ({
  setCashierDiscountsAction: (...a: unknown[]) => setCashierDiscountsAction(...a),
}));

import { CashierDiscountsSetting } from "./CashierDiscountsSetting";

describe("CashierDiscountsSetting", () => {
  it("saves the switch and shows the saved state", async () => {
    setCashierDiscountsAction.mockResolvedValue({ ok: true, enabled: true });
    const user = userEvent.setup();
    render(<CashierDiscountsSetting storeName="The Bondi Store" initial={false} />);
    const toggle = screen.getByRole("switch", { name: /cashiers can give discounts/i });
    expect(toggle).toHaveAttribute("aria-checked", "false");
    await user.click(toggle);
    expect(setCashierDiscountsAction).toHaveBeenCalledWith(true);
    await waitFor(() => expect(toggle).toHaveAttribute("aria-checked", "true"));
  });

  it("puts the switch back and says so when saving fails", async () => {
    setCashierDiscountsAction.mockResolvedValue({ ok: false, message: "You do not have permission." });
    const user = userEvent.setup();
    render(<CashierDiscountsSetting storeName="The Bondi Store" initial={false} />);
    const toggle = screen.getByRole("switch", { name: /cashiers can give discounts/i });
    await user.click(toggle);
    expect(await screen.findByText("You do not have permission.")).toBeInTheDocument();
    expect(toggle).toHaveAttribute("aria-checked", "false");
  });
});
