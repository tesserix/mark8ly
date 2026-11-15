import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

const setInStoreSettingAction = vi.fn();
vi.mock("@/app/(admin)/settings/in-store/actions", () => ({
  setInStoreSettingAction: (...a: unknown[]) => setInStoreSettingAction(...a),
}));

import { InStoreSettingsForm } from "./InStoreSettingsForm";

const initial = { in_store_enabled: false, cashier_discounts_enabled: false };
beforeEach(() => setInStoreSettingAction.mockReset());

describe("InStoreSettingsForm", () => {
  it("turns in-store sales on and shows the saved state", async () => {
    setInStoreSettingAction.mockResolvedValue({ ok: true, settings: { in_store_enabled: true, cashier_discounts_enabled: false } });
    const user = userEvent.setup();
    render(<InStoreSettingsForm storeName="The Bondi Store" initial={initial} />);
    const sell = screen.getByRole("switch", { name: /sell in store/i });
    await user.click(sell);
    expect(setInStoreSettingAction).toHaveBeenCalledWith({ in_store_enabled: true });
    await waitFor(() => expect(sell).toHaveAttribute("aria-checked", "true"));
  });

  it("only offers the cashier discount switch while in-store sales are on", () => {
    const { rerender } = render(<InStoreSettingsForm storeName="S" initial={initial} />);
    expect(screen.queryByRole("switch", { name: /cashiers can give discounts/i })).not.toBeInTheDocument();
    rerender(<InStoreSettingsForm storeName="S" initial={{ ...initial, in_store_enabled: true }} key="on" />);
    expect(screen.getByRole("switch", { name: /cashiers can give discounts/i })).toBeInTheDocument();
  });

  it("puts the switch back and says so when saving fails", async () => {
    setInStoreSettingAction.mockResolvedValue({ ok: false, message: "Only the owner or an admin can change this." });
    const user = userEvent.setup();
    render(<InStoreSettingsForm storeName="S" initial={initial} />);
    const sell = screen.getByRole("switch", { name: /sell in store/i });
    await user.click(sell);
    expect(await screen.findByText("Only the owner or an admin can change this.")).toBeInTheDocument();
    expect(sell).toHaveAttribute("aria-checked", "false");
  });
});
