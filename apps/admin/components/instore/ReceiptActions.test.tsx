import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

const receiptAction = vi.fn();
vi.mock("@/app/(admin)/sell/actions", () => ({ receiptAction: (...a: unknown[]) => receiptAction(...a) }));

import { ReceiptActions } from "./ReceiptActions";

const open = vi.fn();
beforeEach(() => {
  receiptAction.mockReset();
  open.mockReset();
  vi.stubGlobal("open", open);
});

describe("ReceiptActions", () => {
  it("emails the receipt and confirms where it went", async () => {
    receiptAction.mockResolvedValue({ ok: true, url: "https://s.mark8ly.com/r/tok" });
    const user = userEvent.setup();
    render(<ReceiptActions storeId="s1" orderId="o1" />);
    await user.type(screen.getByRole("textbox", { name: /customer.s email/i }), "jo@example.com");
    await user.click(screen.getByRole("button", { name: "Email receipt" }));
    expect(receiptAction).toHaveBeenCalledWith("s1", "o1", { send: true, email: "jo@example.com" });
    expect(await screen.findByText("Receipt sent to jo@example.com.")).toBeInTheDocument();
  });

  it("keeps Email receipt off until the address looks like one", async () => {
    const user = userEvent.setup();
    render(<ReceiptActions storeId="s1" orderId="o1" />);
    const send = screen.getByRole("button", { name: "Email receipt" });
    expect(send).toBeDisabled();
    await user.type(screen.getByRole("textbox", { name: /customer.s email/i }), "jo@");
    expect(send).toBeDisabled();
  });

  it("opens the receipt to print", async () => {
    receiptAction.mockResolvedValue({ ok: true, url: "https://s.mark8ly.com/r/tok" });
    const user = userEvent.setup();
    render(<ReceiptActions storeId="s1" orderId="o1" />);
    await user.click(screen.getByRole("button", { name: "Print receipt" }));
    expect(receiptAction).toHaveBeenCalledWith("s1", "o1", { send: false });
    expect(open).toHaveBeenCalledWith("https://s.mark8ly.com/r/tok", "_blank", "noopener");
  });

  it("shows the server's message when it fails", async () => {
    receiptAction.mockResolvedValue({ ok: false, message: "Enter a valid email address" });
    const user = userEvent.setup();
    render(<ReceiptActions storeId="s1" orderId="o1" />);
    await user.click(screen.getByRole("button", { name: "Print receipt" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Enter a valid email address");
    expect(open).not.toHaveBeenCalled();
  });
});
