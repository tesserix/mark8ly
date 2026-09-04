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

async function submitExistingAccount(password = "correct-horse-battery") {
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
      password: "correct-horse-battery",
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
      "correct-horse-battery",
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
    await userEvent.type(screen.getByLabelText(/^create password$/i), "pw-123456789");
    await userEvent.type(
      screen.getByLabelText(/^confirm password$/i),
      "pw-123456789",
    );
    await userEvent.click(
      screen.getByRole("button", { name: /create account and join store/i }),
    );

    await waitFor(() =>
      expect(signUp).toHaveBeenCalledWith("staff@example.com", "pw-123456789"),
    );
    expect(acceptInviteWithZitadel).not.toHaveBeenCalled();
  });

  it("still offers the Google button", () => {
    render(<AcceptInviteForm token="invite-token" invitation={invitation} />);

    expect(
      screen.getByRole("button", { name: /continue with google/i }),
    ).toBeTruthy();
  });
});
