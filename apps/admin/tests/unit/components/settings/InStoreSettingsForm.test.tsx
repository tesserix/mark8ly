import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

const setAction = vi.fn();
vi.mock("@/app/(admin)/settings/in-store/actions", () => ({ setInStoreSettingAction: (p: unknown) => setAction(p) }));

import { InStoreSettingsForm } from "@/components/settings/InStoreSettingsForm";

const on = { cashier_discounts_enabled: false, in_store_enabled: true, abn: "" };

describe("InStoreSettingsForm ABN (#1108)", () => {
  it("saves an AU store's ABN", async () => {
    setAction.mockResolvedValue({ ok: true, settings: { ...on, abn: "51824753556" } });
    render(<InStoreSettingsForm storeName="Bondi" countryCode="AU" initial={on} />);
    fireEvent.change(screen.getByLabelText(/ABN/), { target: { value: "51 824 753 556" } });
    fireEvent.click(screen.getByRole("button", { name: /save abn/i }));
    await waitFor(() => expect(setAction).toHaveBeenCalledWith({ abn: "51 824 753 556" }));
    expect(await screen.findByDisplayValue("51 824 753 556")).toBeInTheDocument();
  });

  it("shows the API's refusal", async () => {
    setAction.mockResolvedValue({ ok: false, message: "that is not a valid ABN; check the digits" });
    render(<InStoreSettingsForm storeName="Bondi" countryCode="AU" initial={on} />);
    fireEvent.change(screen.getByLabelText(/ABN/), { target: { value: "1" } });
    fireEvent.click(screen.getByRole("button", { name: /save abn/i }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/not a valid ABN/);
  });

  it("has no ABN field outside Australia", () => {
    render(<InStoreSettingsForm storeName="Sugar" countryCode="IN" initial={on} />);
    expect(screen.queryByLabelText(/ABN/)).not.toBeInTheDocument();
  });
});
