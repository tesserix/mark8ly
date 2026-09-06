import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";

// Critical 1 fix (whole-branch review, phase 3a): page.tsx computed
// `isZitadel` from publicConfig.authProvider but never passed it to
// SignInForm, so the Zitadel path was unreachable regardless of the
// flag. This pins that the resolved provider actually reaches the form.

vi.mock("@repo/ui/brand-bar", () => ({
  BrandBar: () => <div data-testid="brand-bar" />,
}));

vi.mock("@/components/auth/SignInForm", () => ({
  SignInForm: (props: {
    provider?: string;
    googleErrorCode?: string;
    initialChallenge?: string;
    initialMultipleTenants?: boolean;
  }) => (
    <div
      data-testid="sign-in-form"
      data-provider={props.provider ?? ""}
      data-google-error={props.googleErrorCode ?? ""}
      data-initial-challenge={props.initialChallenge ?? ""}
      data-initial-multi={props.initialMultipleTenants ? "1" : "0"}
    />
  ),
}));

const redirectMock = vi.fn((dest: string) => {
  // Stand-in for next/navigation's redirect, which throws to unwind the
  // render. Throwing here keeps the control flow honest: anything after
  // the redirect call must not run.
  throw new Error(`REDIRECT:${dest}`);
});
vi.mock("next/navigation", () => ({
  redirect: (dest: string) => redirectMock(dest),
}));

const loginErrorCookie: { value?: string } = {};
vi.mock("next/headers", () => ({
  cookies: async () => ({
    get: (name: string) =>
      name === "zt_login_error" && loginErrorCookie.value !== undefined
        ? { name, value: loginErrorCookie.value }
        : undefined,
  }),
}));

vi.mock("@/lib/auth/sanitize-return-url", () => ({
  sanitizeReturnUrl: (v: string | null | undefined) => v ?? undefined,
}));

const configMock = vi.hoisted(() => ({ authProvider: "gip" as "gip" | "zitadel" }));
vi.mock("@/lib/config", () => ({ publicConfig: configMock }));

import LoginPage from "./page";
import { beforeEach } from "vitest";

beforeEach(() => {
  vi.clearAllMocks();
  delete loginErrorCookie.value;
});

/** Runs the page and returns the destination its redirect threw with. */
async function redirectDestination(
  searchParams: Record<string, string>,
): Promise<string> {
  await expect(
    LoginPage({ searchParams: Promise.resolve(searchParams) }),
  ).rejects.toThrow(/^REDIRECT:/);
  return redirectMock.mock.calls[0]![0];
}

describe("LoginPage — provider wiring", () => {
  it("passes provider=\"gip\" to SignInForm when the flag is unset", async () => {
    configMock.authProvider = "gip";

    const element = await LoginPage({ searchParams: Promise.resolve({}) });
    render(element);

    expect(screen.getByTestId("sign-in-form").dataset.provider).toBe("gip");
  });

  it("passes provider=\"zitadel\" to SignInForm once the flag selects it and an authRequest is present", async () => {
    configMock.authProvider = "zitadel";

    const element = await LoginPage({
      searchParams: Promise.resolve({ authRequest: "V2_abc" }),
    });
    render(element);

    expect(screen.getByTestId("sign-in-form").dataset.provider).toBe("zitadel");
  });
});

// Production report: a failed Google attempt sent the merchant back to
// /login carrying the auth request idp/complete had already SPENT, so the
// "sign in with your email and password instead" advice ended in a raw
// Zitadel body — {"error": "No valid authentication request found"}. The
// finish route now sends the `recovery` sentinel instead, and this page
// has to give it its documented meaning.
describe("LoginPage — recovery sentinel", () => {
  it("bounces a recovery arrival to /login/authorize for a FRESH auth request instead of rendering a dead one", async () => {
    configMock.authProvider = "zitadel";

    const dest = await redirectDestination({
      authRequest: "recovery",
      error: "no_admin_account",
    });

    expect(dest.startsWith("/login/authorize")).toBe(true);
  });

  it("preserves the error code across that bounce so the merchant still learns why Google failed", async () => {
    configMock.authProvider = "zitadel";

    const dest = await redirectDestination({
      authRequest: "recovery",
      error: "no_admin_account",
    });

    expect(new URLSearchParams(dest.split("?")[1]).get("error")).toBe("no_admin_account");
  });

  it("preserves returnUrl alongside the error", async () => {
    configMock.authProvider = "zitadel";

    const dest = await redirectDestination({
      authRequest: "recovery",
      error: "step_up_unsupported",
      returnUrl: "/orders",
    });

    const params = new URLSearchParams(dest.split("?")[1]);
    expect(params.get("returnUrl")).toBe("/orders");
    expect(params.get("error")).toBe("step_up_unsupported");
  });

  it("also carries the error when there is no authRequest at all", async () => {
    configMock.authProvider = "zitadel";

    const dest = await redirectDestination({ error: "email_ambiguous" });

    expect(new URLSearchParams(dest.split("?")[1]).get("error")).toBe("email_ambiguous");
  });

  it("renders the form (no bounce) once a real auth request has been minted", async () => {
    configMock.authProvider = "zitadel";

    const element = await LoginPage({
      searchParams: Promise.resolve({ authRequest: "V2_fresh" }),
    });
    render(element);

    expect(redirectMock).not.toHaveBeenCalled();
    expect(screen.getByTestId("sign-in-form")).toBeTruthy();
  });

  it("reads the error back off the cookie that survived the Zitadel hop", async () => {
    configMock.authProvider = "zitadel";
    loginErrorCookie.value = "no_admin_account";

    const element = await LoginPage({
      searchParams: Promise.resolve({ authRequest: "V2_fresh" }),
    });
    render(element);

    expect(screen.getByTestId("sign-in-form").dataset.googleError).toBe("no_admin_account");
  });

  it("hands an unrecognised code to SignInForm as a code, never as rendered text — messageForAdminGoogleError maps it to the generic message", async () => {
    configMock.authProvider = "zitadel";

    const element = await LoginPage({
      searchParams: Promise.resolve({
        authRequest: "V2_fresh",
        error: "No valid authentication request found",
      }),
    });
    render(element);

    // The value reaches the form only as an opaque code; the form maps
    // anything unrecognised onto a generic message (pinned in
    // lib/auth/google-sign-in-admin.test.ts), so no provider text is
    // rendered. Nothing about the raw string is echoed into the page.
    expect(document.body.textContent).not.toContain("No valid authentication request found");
  });
});

describe("LoginPage — email-OTP challenge arrival (#686)", () => {
  it("passes challenge=email_otp down to the form so it opens on the code step", async () => {
    configMock.authProvider = "zitadel";

    const element = await LoginPage({
      searchParams: Promise.resolve({ authRequest: "ar-1", challenge: "email_otp" }),
    });
    render(element);

    expect(screen.getByTestId("sign-in-form").dataset.initialChallenge).toBe("email_otp");
  });

  it("ignores any other challenge value — mfa/totp must never be openable from a query string", async () => {
    configMock.authProvider = "zitadel";

    for (const challenge of ["mfa", "totp", "email_otp_but_not"]) {
      const element = await LoginPage({
        searchParams: Promise.resolve({ authRequest: "ar-1", challenge }),
      });
      const { unmount } = render(element);
      expect(screen.getByTestId("sign-in-form").dataset.initialChallenge).toBe("");
      unmount();
    }
  });

  it("passes multi=1 through as initialMultipleTenants", async () => {
    configMock.authProvider = "zitadel";

    const element = await LoginPage({
      searchParams: Promise.resolve({
        authRequest: "ar-1",
        challenge: "email_otp",
        multi: "1",
      }),
    });
    render(element);

    expect(screen.getByTestId("sign-in-form").dataset.initialMulti).toBe("1");
  });
});
