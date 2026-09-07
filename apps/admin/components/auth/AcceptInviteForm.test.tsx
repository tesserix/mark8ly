import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

// Issue #679 — pins the accept-invite form:
//   1. The password goes to acceptInviteWithZitadel.
//   2. The browser is handed to the Zitadel login flow by full-page nav.
//   3. provisioning_failed renders as something the invitee can act on.
//   4. There is no Google button — that path was GIP end to end.

const acceptInviteWithZitadel = vi.fn();

vi.mock("@/app/accept-invite/actions", () => ({
  acceptInviteWithZitadel: (...args: unknown[]) =>
    acceptInviteWithZitadel(...args),
}));

import { AcceptInviteForm } from "./AcceptInviteForm";

const invitation = {
  email: "staff@example.com",
  role: "staff",
  tenant_slug: "bondi-store",
  tenant_name: "The Bondi Store",
} as never;

const assign = vi.fn();

beforeEach(() => {
  vi.clearAllMocks();
  acceptInviteWithZitadel.mockResolvedValue({
    ok: true,
    tenantId: "tenant-1",
    signInUrl: "/login/authorize?returnUrl=%2Fdashboard",
  });
  assign.mockReset();
  Object.defineProperty(window, "location", {
    writable: true,
    value: { assign, origin: "https://admin.mark8ly.com" },
  });
});

async function submitExistingAccount(password = "Not-A-Real-Password-1!") {
  await userEvent.type(screen.getByLabelText(/^password$/i), password);
  await userEvent.click(screen.getByRole("button", { name: /accept invite|sign in and accept/i }));
}

describe("AcceptInviteForm", () => {
  it("sends the password to acceptInviteWithZitadel", async () => {
    render(
      <AcceptInviteForm token="invite-token" invitation={invitation} />,
    );

    await submitExistingAccount();

    await waitFor(() =>
      expect(acceptInviteWithZitadel).toHaveBeenCalledTimes(1),
    );
    expect(acceptInviteWithZitadel).toHaveBeenCalledWith({
      token: "invite-token",
      email: "staff@example.com",
      password: "Not-A-Real-Password-1!",
    });
  });

  it("hands the browser to the Zitadel login flow with a full-page navigation", async () => {
    render(
      <AcceptInviteForm token="invite-token" invitation={invitation} />,
    );

    await submitExistingAccount();

    await waitFor(() =>
      expect(assign).toHaveBeenCalledWith("/login/authorize?returnUrl=%2Fdashboard"),
    );
    // /login/authorize is a Route Handler that writes cookies and 302s
    // off-origin — a client-side navigation would never reach it.
  });

  it("renders a provisioning_failed message the invitee can act on", async () => {
    acceptInviteWithZitadel.mockResolvedValue({
      ok: false,
      code: "provisioning_failed",
      message:
        "we couldn't finish setting up your account — please try the invitation link again",
    });

    render(
      <AcceptInviteForm token="invite-token" invitation={invitation} />,
    );

    await submitExistingAccount();

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/invitation link again/i);
    expect(assign).not.toHaveBeenCalled();
  });

  // The exact password from the production incident: eleven characters,
  // which the old `min(8)` schema accepted and Zitadel's 12-character
  // policy then rejected with a message that explained nothing.
  it("rejects the 11-character password that failed in production, before submitting", async () => {
    render(
      <AcceptInviteForm token="invite-token" invitation={invitation} />,
    );

    await submitExistingAccount("Test@123_01");

    expect(
      await screen.findByText(/at least 12 characters/i),
    ).toBeTruthy();
    // The whole point: it never reaches the server.
    expect(acceptInviteWithZitadel).not.toHaveBeenCalled();
  });

  it("shows the full requirements before the first submit", () => {
    render(
      <AcceptInviteForm token="invite-token" invitation={invitation} />,
    );

    const hint = screen.getByText(/at least 12 characters/i);
    expect(hint).toBeTruthy();
    expect(hint.textContent).toMatch(/uppercase/i);
    expect(hint.textContent).toMatch(/lowercase/i);
    expect(hint.textContent).toMatch(/number/i);
    expect(hint.textContent).toMatch(/symbol/i);
  });

  it("renders the server's specific policy error on the password field", async () => {
    // Client validation and the server's policy can drift; the server is
    // authoritative, so its message has to be renderable even for a
    // password the client accepted.
    acceptInviteWithZitadel.mockResolvedValue({
      ok: false,
      code: "password_policy",
      message: "That password needs a symbol, for example ! ? @ or #.",
    });

    render(
      <AcceptInviteForm token="invite-token" invitation={invitation} />,
    );

    await submitExistingAccount();

    const error = await screen.findByRole("alert");
    expect(error).toHaveTextContent(/needs a symbol/i);
    expect(error.id).toBe("invite-password-error");
    expect(assign).not.toHaveBeenCalled();
  });

  it("offers no Google button — that path was GIP end to end", () => {
    render(
      <AcceptInviteForm token="invite-token" invitation={invitation} />,
    );

    expect(
      screen.queryByRole("button", { name: /continue with google/i }),
    ).toBeNull();
  });
});
