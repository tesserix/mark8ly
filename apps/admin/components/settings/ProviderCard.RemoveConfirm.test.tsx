import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ProviderCard } from "./ProviderCard";

// The remove flow used to confirm by RELABELLING the button in place
// ("Remove" → "Confirm remove"). A single click then looked like nothing had
// happened, which cost a real round trip. These tests pin the modal contract:
// the first click must not remove anything, and it must say out loud what is
// about to be removed.

function renderCard(onRemove = vi.fn()) {
  render(
    <ProviderCard
      providerName="stripe"
      isActive
      maskedKey="sk_test_••••1234"
      mode="test"
      onRemove={onRemove}
    />,
  );
  return onRemove;
}

describe("ProviderCard — remove confirmation", () => {
  it("first click opens a confirmation naming the provider, and removes nothing", async () => {
    const user = userEvent.setup();
    const onRemove = renderCard();

    await user.click(screen.getByRole("button", { name: "Remove" }));

    expect(screen.getByText("Remove Stripe?")).toBeInTheDocument();
    expect(onRemove).not.toHaveBeenCalled();
  });

  it("confirming in the dialog calls onRemove", async () => {
    const user = userEvent.setup();
    const onRemove = renderCard(vi.fn().mockResolvedValue({ ok: true }));

    await user.click(screen.getByRole("button", { name: "Remove" }));
    // The dialog's own confirm button — distinct from the card's trigger,
    // which is why the dialog heading is asserted above first.
    const buttons = screen.getAllByRole("button", { name: "Remove" });
    await user.click(buttons[buttons.length - 1]);

    expect(onRemove).toHaveBeenCalledTimes(1);
  });

  it("cancelling closes the dialog without removing", async () => {
    const user = userEvent.setup();
    const onRemove = renderCard();

    await user.click(screen.getByRole("button", { name: "Remove" }));
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.queryByText("Remove Stripe?")).not.toBeInTheDocument();
    expect(onRemove).not.toHaveBeenCalled();
  });

  it("a failed removal surfaces the reason instead of looking successful", async () => {
    const user = userEvent.setup();
    const onRemove = renderCard(
      vi.fn().mockResolvedValue({ ok: false, message: "Unauthorized." }),
    );

    await user.click(screen.getByRole("button", { name: "Remove" }));
    const buttons = screen.getAllByRole("button", { name: "Remove" });
    await user.click(buttons[buttons.length - 1]);

    expect(await screen.findByRole("alert")).toHaveTextContent("Unauthorized.");
    expect(onRemove).toHaveBeenCalledTimes(1);
  });
});
