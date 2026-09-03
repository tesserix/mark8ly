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
  SignInForm: (props: { provider?: string }) => (
    <div data-testid="sign-in-form" data-provider={props.provider ?? ""} />
  ),
}));

vi.mock("@/lib/auth/sanitize-return-url", () => ({
  sanitizeReturnUrl: (v: string | null | undefined) => v ?? undefined,
}));

const configMock = vi.hoisted(() => ({ authProvider: "gip" as "gip" | "zitadel" }));
vi.mock("@/lib/config", () => ({ publicConfig: configMock }));

import LoginPage from "./page";

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
