import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

// Phase 3a — pins the provider branch this task adds to SignInForm:
//   1. provider="zitadel" routes through signInWithZitadel, never GIP.
//   2. provider unset (or anything else) keeps the GIP path — the
//      safety property this whole phase depends on, expressed as a
//      test rather than only a diff inspection.
//   3. A totpRequired result renders the Zitadel TOTP screen and mints
//      no session (no navigation).
//   4. Google/Apple are hidden under Zitadel, present otherwise.

const signInWithPassword = vi.fn();
const signIn = vi.fn();
const signInWithZitadel = vi.fn();
const confirmZitadelTotp = vi.fn();
const push = vi.fn();

vi.mock("@/lib/gip/signup", () => ({
  signInWithPassword: (...args: unknown[]) => signInWithPassword(...args),
  signInWithGoogle: vi.fn(),
  signInWithApple: vi.fn(),
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

vi.mock("@/lib/gip/apple-js", () => ({
  getAppleCredential: vi.fn(),
}));

vi.mock("@/lib/gip/link", () => ({
  linkGoogleToInternalPassword: vi.fn(),
}));

vi.mock("@/app/login/actions", () => ({
  signIn: (...args: unknown[]) => signIn(...args),
  signInWithZitadel: (...args: unknown[]) => signInWithZitadel(...args),
  confirmZitadelTotp: (...args: unknown[]) => confirmZitadelTotp(...args),
  confirmMFALogin: vi.fn(),
  confirmEmailOTPLogin: vi.fn(),
  resendEmailOTPCode: vi.fn(),
}));

const startAdminGoogleSignIn = vi.fn();
vi.mock("@/app/auth/idp/actions", () => ({
  startAdminGoogleSignIn: (...args: unknown[]) => startAdminGoogleSignIn(...args),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push }),
}));

vi.mock("@/lib/auth/cross-domain-handoff", () => ({
  prepareCrossDomainNavigation: vi.fn(async () => ({ kind: "same-origin" })),
}));

vi.mock("@/lib/config", () => ({
  appleSignInEnabled: false,
  publicConfig: { zitadelIssuer: "https://auth.tesserix.app" },
}));

import { SignInForm } from "./SignInForm";
import { signInWithGoogle } from "@/lib/gip/signup";
import { getGoogleCredential } from "@/lib/gip/google-gsi";

const signInWithGoogleMock = vi.mocked(signInWithGoogle);
const getGoogleCredentialMock = vi.mocked(getGoogleCredential);

const assign = vi.fn();

beforeEach(() => {
  vi.clearAllMocks();
  signInWithPassword.mockResolvedValue({ idToken: "id-token", uid: "uid-1" });
  signIn.mockResolvedValue({
    ok: true,
    data: { multipleTenants: false, mfaRequired: false, emailOtpRequired: false },
  });
  assign.mockReset();
  Object.defineProperty(window, "location", {
    writable: true,
    value: { assign, origin: "https://admin.mark8ly.com" },
  });
});

async function fillAndSubmit() {
  await userEvent.type(screen.getByLabelText(/email address/i), "founder@example.com");
  await userEvent.type(screen.getByLabelText(/^password$/i), "correct-horse");
  await userEvent.click(screen.getByRole("button", { name: /^sign in$/i }));
}

describe("SignInForm — provider branch", () => {
  it("with provider=zitadel, submits through signInWithZitadel and never touches GIP signInWithPassword", async () => {
    signInWithZitadel.mockResolvedValue({
      ok: true,
      data: { multipleTenants: false, mfaRequired: false, emailOtpRequired: false },
    });

    render(<SignInForm provider="zitadel" authRequestId="req-1" />);
    await fillAndSubmit();

    await waitFor(() => expect(signInWithZitadel).toHaveBeenCalledTimes(1));
    expect(signInWithZitadel).toHaveBeenCalledWith({
      email: "founder@example.com",
      password: "correct-horse",
      authRequestId: "req-1",
    });
    expect(signInWithPassword).not.toHaveBeenCalled();
    expect(signIn).not.toHaveBeenCalled();
  });

  it("with provider unset, submits through the GIP path and never calls signInWithZitadel", async () => {
    render(<SignInForm />);
    await fillAndSubmit();

    await waitFor(() => expect(signInWithPassword).toHaveBeenCalledTimes(1));
    expect(signIn).toHaveBeenCalledWith({ idToken: "id-token", uid: "uid-1" });
    expect(signInWithZitadel).not.toHaveBeenCalled();
  });

  it("with an unrecognised provider value, still submits through the GIP path", async () => {
    render(<SignInForm provider="gip" />);
    await fillAndSubmit();

    await waitFor(() => expect(signInWithPassword).toHaveBeenCalledTimes(1));
    expect(signInWithZitadel).not.toHaveBeenCalled();
  });

  it("a totpRequired result renders the TOTP screen and mints no session", async () => {
    signInWithZitadel.mockResolvedValue({
      ok: true,
      data: {
        multipleTenants: false,
        mfaRequired: false,
        emailOtpRequired: false,
        totpRequired: true,
        zitadelSessionId: "sess-1",
        zitadelSessionToken: "tok-1",
        zitadelTenantCode: "code-1",
      },
    });

    render(<SignInForm provider="zitadel" authRequestId="req-1" />);
    await fillAndSubmit();

    // The TOTP screen replaces the credential form.
    await waitFor(() =>
      expect(screen.getByText(/two-factor check/i)).toBeInTheDocument(),
    );
    expect(screen.queryByLabelText(/email address/i)).not.toBeInTheDocument();

    // No session was minted: no navigation, no cross-domain handoff call.
    expect(push).not.toHaveBeenCalled();
    expect(confirmZitadelTotp).not.toHaveBeenCalled();
  });

  it("shows Google but hides Apple when provider=zitadel", () => {
    render(<SignInForm provider="zitadel" authRequestId="req-1" />);
    expect(
      screen.getByRole("button", { name: /continue with google/i }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /continue with apple/i }),
    ).not.toBeInTheDocument();
  });

  it("shows the Google button when provider is not zitadel", () => {
    render(<SignInForm />);
    expect(
      screen.getByRole("button", { name: /continue with google/i }),
    ).toBeInTheDocument();
  });
});

describe("SignInForm — Google through Zitadel", () => {
  it("with provider=zitadel, clicking Google calls startAdminGoogleSignIn and navigates to the returned authUrl, never touching GIP's getGoogleCredential", async () => {
    startAdminGoogleSignIn.mockResolvedValue({
      ok: true,
      authUrl: "https://zitadel.example/idp/authorize?intent=1",
    });

    render(<SignInForm provider="zitadel" authRequestId="req-1" />);
    await userEvent.click(screen.getByRole("button", { name: /continue with google/i }));

    await waitFor(() => expect(startAdminGoogleSignIn).toHaveBeenCalledWith("req-1"));
    await waitFor(() =>
      expect(assign).toHaveBeenCalledWith("https://zitadel.example/idp/authorize?intent=1"),
    );
    expect(signInWithGoogle).not.toHaveBeenCalled();
  });

  it("with provider unset, clicking Google never calls startAdminGoogleSignIn", async () => {
    getGoogleCredentialMock.mockResolvedValue({ credential: "cred-1" });
    signInWithGoogleMock.mockResolvedValue({ kind: "complete", idToken: "id-1", uid: "uid-1" });

    render(<SignInForm />);
    await userEvent.click(screen.getByRole("button", { name: /continue with google/i }));

    await waitFor(() => expect(signInWithGoogleMock).toHaveBeenCalled());
    expect(startAdminGoogleSignIn).not.toHaveBeenCalled();
  });

  it("renders a truthful, distinct message for a no_admin_account error and never suggests retrying", () => {
    render(
      <SignInForm
        provider="zitadel"
        authRequestId="req-1"
        googleErrorCode="no_admin_account"
      />,
    );

    const message = screen.getByRole("alert").textContent ?? "";
    expect(message.toLowerCase()).toContain("no admin account");
    expect(message.toLowerCase()).not.toMatch(/try again|retry/);
  });

  it("shows a generic error and stays on the form when startAdminGoogleSignIn fails", async () => {
    startAdminGoogleSignIn.mockResolvedValue({
      ok: false,
      message: "Google sign-in is temporarily unavailable. Please try again shortly.",
    });

    render(<SignInForm provider="zitadel" authRequestId="req-1" />);
    await userEvent.click(screen.getByRole("button", { name: /continue with google/i }));

    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toMatch(/temporarily unavailable/i),
    );
    expect(assign).not.toHaveBeenCalled();
  });
});

describe("SignInForm — Zitadel callbackUrl handoff (Important 1)", () => {
  it("a complete outcome with a callbackUrl navigates there instead of straight to the dashboard", async () => {
    signInWithZitadel.mockResolvedValue({
      ok: true,
      data: {
        multipleTenants: false,
        mfaRequired: false,
        emailOtpRequired: false,
        callbackUrl: "https://admin.mark8ly.com/auth/callback?state=abc",
      },
    });

    render(<SignInForm provider="zitadel" authRequestId="req-1" />);
    await fillAndSubmit();

    await waitFor(() =>
      expect(assign).toHaveBeenCalledWith(
        "https://admin.mark8ly.com/auth/callback?state=abc",
      ),
    );
    expect(push).not.toHaveBeenCalled();
  });

  it("a complete outcome with no callbackUrl falls back to the existing destination logic", async () => {
    signInWithZitadel.mockResolvedValue({
      ok: true,
      data: { multipleTenants: false, mfaRequired: false, emailOtpRequired: false },
    });

    render(<SignInForm provider="zitadel" authRequestId="req-1" />);
    await fillAndSubmit();

    await waitFor(() => expect(push).toHaveBeenCalledWith("/dashboard"));
    expect(assign).not.toHaveBeenCalled();
  });

  it("refuses to navigate to a callbackUrl on a foreign origin, and still completes login via the normal destination", async () => {
    const hostileUrl = "https://evil.example.com/steal?state=abc";
    signInWithZitadel.mockResolvedValue({
      ok: true,
      data: {
        multipleTenants: false,
        mfaRequired: false,
        emailOtpRequired: false,
        callbackUrl: hostileUrl,
      },
    });

    render(<SignInForm provider="zitadel" authRequestId="req-1" />);
    await fillAndSubmit();

    // Login still completes — the session is already valid, so a
    // rejected callbackUrl must not strand the merchant.
    await waitFor(() => expect(push).toHaveBeenCalledWith("/dashboard"));
    expect(assign).not.toHaveBeenCalledWith(hostileUrl);
    expect(assign).not.toHaveBeenCalled();
  });
});

describe("SignInForm — handoffUrl validation (Minor)", () => {
  it("navigates to a handoffUrl on Zitadel's own hosted-login origin", async () => {
    signInWithZitadel.mockResolvedValue({
      ok: true,
      data: {
        multipleTenants: false,
        mfaRequired: false,
        emailOtpRequired: false,
        handoffUrl: "https://auth.tesserix.app/ui/v2/login",
      },
    });

    render(<SignInForm provider="zitadel" authRequestId="req-1" />);
    await fillAndSubmit();

    await waitFor(() =>
      expect(assign).toHaveBeenCalledWith("https://auth.tesserix.app/ui/v2/login"),
    );
  });

  it("refuses to navigate to a handoffUrl on an untrusted origin", async () => {
    signInWithZitadel.mockResolvedValue({
      ok: true,
      data: {
        multipleTenants: false,
        mfaRequired: false,
        emailOtpRequired: false,
        handoffUrl: "https://evil.example.com/steal",
      },
    });

    render(<SignInForm provider="zitadel" authRequestId="req-1" />);
    await fillAndSubmit();

    await waitFor(() => expect(signInWithZitadel).toHaveBeenCalledTimes(1));
    expect(assign).not.toHaveBeenCalled();
  });
});
