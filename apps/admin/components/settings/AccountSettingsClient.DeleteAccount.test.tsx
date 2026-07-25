/**
 * DeleteAccountSection unit tests — Vitest + Testing Library.
 *
 * DeleteAccountSection mirrors the DangerZone "reset my profile" flow but
 * calls deleteTenantAccountAction (full tenant-account deletion) instead,
 * and renders unconditionally (no `editable` gate) with owner-aware copy.
 *
 * Cases:
 *   1. Reveal confirm → type DELETE → click confirm → action called with "DELETE"
 *   2. Typing "delete" (wrong case) keeps confirm button disabled → action NOT called
 *   3. Typing unrelated text keeps confirm button disabled → action NOT called
 *   4. Owner vs non-owner copy differs
 *   5. Success (ok: true) → window.location.href set to "/logout"
 *   6. Failure (ok: false) → toast.error called, no navigation
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

// ---------------------------------------------------------------------------
// Mocks — declared before component imports
// ---------------------------------------------------------------------------

const mockToastSuccess = vi.fn();
const mockToastError = vi.fn();

vi.mock("@/components/feedback/Toaster", () => ({
  useToast: () => ({
    toast: {
      success: mockToastSuccess,
      error: mockToastError,
      info: vi.fn(),
      warning: vi.fn(),
      dismiss: vi.fn(),
    },
  }),
}));

const mockDeleteTenantAccountAction = vi.fn();

vi.mock("@/app/(admin)/settings/actions", () => ({
  deleteTenantAccountAction: (...args: unknown[]) =>
    mockDeleteTenantAccountAction(...args),
}));

// ---------------------------------------------------------------------------
// Import after mocks
// ---------------------------------------------------------------------------

import { DeleteAccountSection } from "./AccountSettingsClient";

// ---------------------------------------------------------------------------
// beforeEach
// ---------------------------------------------------------------------------

beforeEach(() => {
  mockToastSuccess.mockReset();
  mockToastError.mockReset();
  mockDeleteTenantAccountAction.mockReset();
  mockDeleteTenantAccountAction.mockResolvedValue({ ok: true });

  Object.defineProperty(window, "location", {
    writable: true,
    value: { href: "" },
  });
});

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("DeleteAccountSection", () => {
  it("reveal confirm, type DELETE, click confirm — action called with 'DELETE'", async () => {
    const user = userEvent.setup();
    render(<DeleteAccountSection isOwner={true} />);

    await user.click(screen.getByRole("button", { name: /delete store/i }));

    const input = screen.getByPlaceholderText("DELETE");
    await user.type(input, "DELETE");

    await user.click(
      screen.getByRole("button", { name: /delete store and sign out/i }),
    );

    await waitFor(() => {
      expect(mockDeleteTenantAccountAction).toHaveBeenCalledWith("DELETE");
    });
  });

  it("wrong-case 'delete' keeps confirm button disabled — action NOT called", async () => {
    const user = userEvent.setup();
    render(<DeleteAccountSection isOwner={true} />);

    await user.click(screen.getByRole("button", { name: /delete store/i }));

    const input = screen.getByPlaceholderText("DELETE");
    await user.type(input, "delete");

    const confirmButton = screen.getByRole("button", {
      name: /delete store and sign out/i,
    });
    expect(confirmButton).toBeDisabled();

    await user.click(confirmButton);
    expect(mockDeleteTenantAccountAction).not.toHaveBeenCalled();
  });

  it("unrelated text keeps confirm button disabled — action NOT called", async () => {
    const user = userEvent.setup();
    render(<DeleteAccountSection isOwner={false} />);

    await user.click(screen.getByRole("button", { name: /remove my access/i }));

    const input = screen.getByPlaceholderText("DELETE");
    await user.type(input, "please delete my account");

    const confirmButton = screen.getByRole("button", {
      name: /remove my access and sign out/i,
    });
    expect(confirmButton).toBeDisabled();

    await user.click(confirmButton);
    expect(mockDeleteTenantAccountAction).not.toHaveBeenCalled();
  });

  it("owner copy warns about permanent store deletion", () => {
    render(<DeleteAccountSection isOwner={true} />);

    expect(
      screen.getByText(/permanently deletes the entire store and all its data/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /^delete store$/i }),
    ).toBeInTheDocument();
  });

  it("non-owner copy warns about losing access only, store unaffected", () => {
    render(<DeleteAccountSection isOwner={false} />);

    expect(
      screen.getByText(/removes your access to this store/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /^remove my access$/i }),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(/permanently deletes the entire store/i),
    ).not.toBeInTheDocument();
  });

  it("success — window.location.href set to /logout", async () => {
    const user = userEvent.setup();
    mockDeleteTenantAccountAction.mockResolvedValue({ ok: true });

    render(<DeleteAccountSection isOwner={true} />);

    await user.click(screen.getByRole("button", { name: /delete store/i }));
    await user.type(screen.getByPlaceholderText("DELETE"), "DELETE");
    await user.click(
      screen.getByRole("button", { name: /delete store and sign out/i }),
    );

    await waitFor(() => {
      expect(window.location.href).toBe("/logout");
    });
  });

  it("failure — toast.error called, no navigation", async () => {
    const user = userEvent.setup();
    mockDeleteTenantAccountAction.mockResolvedValue({
      ok: false,
      code: "forbidden",
      message: "You do not have permission to perform this action.",
    });

    render(<DeleteAccountSection isOwner={false} />);

    await user.click(screen.getByRole("button", { name: /remove my access/i }));
    await user.type(screen.getByPlaceholderText("DELETE"), "DELETE");
    await user.click(
      screen.getByRole("button", { name: /remove my access and sign out/i }),
    );

    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalledWith(
        "Couldn't delete account",
        "You do not have permission to perform this action.",
      );
    });
    expect(window.location.href).toBe("");
  });
});
