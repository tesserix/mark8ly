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

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push }),
}));

vi.mock("@/lib/auth/cross-domain-handoff", () => ({
  prepareCrossDomainNavigation: vi.fn(async () => ({ kind: "same-origin" })),
}));

import { SignInForm } from "./SignInForm";

beforeEach(() => {
  vi.clearAllMocks();
  signInWithPassword.mockResolvedValue({ idToken: "id-token", uid: "uid-1" });
  signIn.mockResolvedValue({
    ok: true,
    data: { multipleTenants: false, mfaRequired: false, emailOtpRequired: false },
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

  it("hides Google and Apple when provider=zitadel", () => {
    render(<SignInForm provider="zitadel" authRequestId="req-1" />);
    expect(
      screen.queryByRole("button", { name: /continue with google/i }),
    ).not.toBeInTheDocument();
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
