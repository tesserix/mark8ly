import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

// Issue #679 — pins the provider branch on the accept-invite form:
//   1. provider="zitadel" routes through acceptInviteWithZitadel with
//      the password, and touches no GIP helper at all.
//   2. provider unset keeps the GIP flow byte-for-byte — this is a
//      branch, not a replacement.
//   3. provisioning_failed renders as something the invitee can act on.
//   4. The Google button, which authenticates through GIP end to end,
//      is absent on the Zitadel path and present otherwise.

const signUp = vi.fn();
const signInWithPassword = vi.fn();
const acceptInvite = vi.fn();
const acceptInviteWithZitadel = vi.fn();
const push = vi.fn();
const refresh = vi.fn();

vi.mock("@/lib/gip/signup", () => ({
  signUp: (...args: unknown[]) => signUp(...args),
  signInWithPassword: (...args: unknown[]) => signInWithPassword(...args),
  signInWithGoogle: vi.fn(),
  GIPError: class GIPError extends Error {
    code: string;
    constructor(code: string) {
      super(code);
      this.code = code;
    }
  },
}));

vi.mock("@/lib/gip/google-gsi", () => ({
  getGoogleCredential: vi.fn(),
}));

vi.mock("@/app/accept-invite/actions", () => ({
  acceptInvite: (...args: unknown[]) => acceptInvite(...args),
  acceptInviteWithZitadel: (...args: unknown[]) =>
    acceptInviteWithZitadel(...args),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push, refresh }),
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
  signUp.mockResolvedValue({ idToken: "gip-id-token", uid: "gip-uid" });
  signInWithPassword.mockResolvedValue({
    idToken: "gip-id-token",
    uid: "gip-uid",
  });
  acceptInvite.mockResolvedValue({ ok: true, tenantId: "tenant-1" });
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

describe("AcceptInviteForm — Zitadel path", () => {
  it("sends the password to acceptInviteWithZitadel and never calls GIP", async () => {
    render(
      <AcceptInviteForm
        token="invite-token"
        invitation={invitation}
        provider="zitadel"
      />,
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
    // No GIP account creation, no GIP sign-in, and therefore no
    // id_token for the old accept action to forward.
    expect(signUp).not.toHaveBeenCalled();
    expect(signInWithPassword).not.toHaveBeenCalled();
    expect(acceptInvite).not.toHaveBeenCalled();
  });

  it("hands the browser to the Zitadel login flow, not router.push", async () => {
    render(
      <AcceptInviteForm
        token="invite-token"
        invitation={invitation}
        provider="zitadel"
      />,
    );

    await submitExistingAccount();

    await waitFor(() =>
      expect(assign).toHaveBeenCalledWith("/login/authorize?returnUrl=%2Fdashboard"),
    );
    // /login/authorize is a Route Handler that writes cookies and 302s
    // off-origin — a client-side navigation would never reach it.
    expect(push).not.toHaveBeenCalled();
  });

  it("renders a provisioning_failed message the invitee can act on", async () => {
    acceptInviteWithZitadel.mockResolvedValue({
      ok: false,
      code: "provisioning_failed",
      message:
        "we couldn't finish setting up your account — please try the invitation link again",
    });

    render(
      <AcceptInviteForm
        token="invite-token"
        invitation={invitation}
        provider="zitadel"
      />,
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
      <AcceptInviteForm
        token="invite-token"
        invitation={invitation}
        provider="zitadel"
      />,
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
      <AcceptInviteForm
        token="invite-token"
        invitation={invitation}
        provider="zitadel"
      />,
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
      <AcceptInviteForm
        token="invite-token"
        invitation={invitation}
        provider="zitadel"
      />,
    );

    await submitExistingAccount();

    const error = await screen.findByRole("alert");
    expect(error).toHaveTextContent(/needs a symbol/i);
    expect(error.id).toBe("invite-password-error");
    expect(assign).not.toHaveBeenCalled();
  });

  it("offers no Google button — that path is GIP end to end", () => {
    render(
      <AcceptInviteForm
        token="invite-token"
        invitation={invitation}
        provider="zitadel"
      />,
    );

    expect(
      screen.queryByRole("button", { name: /continue with google/i }),
    ).toBeNull();
  });
});

describe("AcceptInviteForm — GIP path (unchanged)", () => {
  it("signs in through GIP and calls the original acceptInvite action", async () => {
    render(<AcceptInviteForm token="invite-token" invitation={invitation} />);

    await submitExistingAccount();

    await waitFor(() => expect(acceptInvite).toHaveBeenCalledTimes(1));
    expect(signInWithPassword).toHaveBeenCalledWith(
      "staff@example.com",
      "Not-A-Real-Password-1!",
    );
    expect(acceptInvite).toHaveBeenCalledWith({
      token: "invite-token",
      idToken: "gip-id-token",
      uid: "gip-uid",
      verifiedEmail: "staff@example.com",
    });
    expect(acceptInviteWithZitadel).not.toHaveBeenCalled();
    await waitFor(() => expect(push).toHaveBeenCalledWith("/dashboard"));
  });

  it("creates a GIP account in create mode", async () => {
    render(<AcceptInviteForm token="invite-token" invitation={invitation} />);

    await userEvent.click(screen.getByRole("tab", { name: /create an account/i }));
    await userEvent.type(screen.getByLabelText(/^create password$/i), "Also-Not-A-Real-Password-2!");
    await userEvent.type(
      screen.getByLabelText(/^confirm password$/i),
      "Also-Not-A-Real-Password-2!",
    );
    await userEvent.click(
      screen.getByRole("button", { name: /create account and join store/i }),
    );

    await waitFor(() =>
      expect(signUp).toHaveBeenCalledWith("staff@example.com", "Also-Not-A-Real-Password-2!"),
    );
    expect(acceptInviteWithZitadel).not.toHaveBeenCalled();
  });

  it("does not hold an existing GIP sign-in password to the Zitadel policy", async () => {
    // GIP's own minimum is 8. A user whose password was set under that
    // rule must still be able to sign in and accept — they cannot change
    // it from this form.
    render(<AcceptInviteForm token="invite-token" invitation={invitation} />);

    await submitExistingAccount("gip-old8");

    await waitFor(() => expect(signInWithPassword).toHaveBeenCalledTimes(1));
    expect(screen.queryByText(/at least 12 characters/i)).toBeNull();
  });

  it("still offers the Google button", () => {
    render(<AcceptInviteForm token="invite-token" invitation={invitation} />);

    expect(
      screen.getByRole("button", { name: /continue with google/i }),
    ).toBeTruthy();
  });
});
