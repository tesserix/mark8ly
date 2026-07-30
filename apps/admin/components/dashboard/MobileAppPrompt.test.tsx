import { describe, it, expect, beforeEach, vi, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { MobileAppPrompt, dismissalKey } from "./MobileAppPrompt";

const TENANT_A = "fb5ceef7-335e-4ebd-96e4-f75ded263c86";
const TENANT_B = "8c302556-b647-4824-8ce4-73f547ca456e";

describe("MobileAppPrompt", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("shows the prompt when it has never been dismissed", async () => {
    render(<MobileAppPrompt tenantId={TENANT_A} />);
    expect(
      await screen.findByText("Take the dashboard with you"),
    ).toBeInTheDocument();
  });

  it("hides the prompt permanently once dismissed", async () => {
    const user = userEvent.setup();
    render(<MobileAppPrompt tenantId={TENANT_A} />);

    await screen.findByText("Take the dashboard with you");
    await user.click(
      screen.getByRole("button", { name: /dismiss the mobile app suggestion/i }),
    );

    await waitFor(() =>
      expect(screen.queryByText("Take the dashboard with you")).toBeNull(),
    );
    expect(window.localStorage.getItem(dismissalKey(TENANT_A))).toBe("1");
  });

  it("stays dismissed across a remount", async () => {
    window.localStorage.setItem(dismissalKey(TENANT_A), "1");
    render(<MobileAppPrompt tenantId={TENANT_A} />);

    // Give the resolving effect a chance to run before asserting absence,
    // otherwise this passes merely because nothing has rendered yet.
    await waitFor(() =>
      expect(screen.queryByText("Take the dashboard with you")).toBeNull(),
    );
  });

  it("scopes dismissal per tenant — dismissing one does not dismiss another", async () => {
    window.localStorage.setItem(dismissalKey(TENANT_A), "1");
    render(<MobileAppPrompt tenantId={TENANT_B} />);
    expect(
      await screen.findByText("Take the dashboard with you"),
    ).toBeInTheDocument();
  });

  it("renders a store badge, not just the copy", async () => {
    render(<MobileAppPrompt tenantId={TENANT_A} />);
    await screen.findByText("Take the dashboard with you");
    // Play is the live platform; iOS is deliberately withheld until approval.
    expect(screen.getByAltText(/Google Play/i)).toBeInTheDocument();
  });

  it("fails open and still shows the prompt when localStorage is unreadable", async () => {
    vi.spyOn(window.localStorage, "getItem").mockImplementation(() => {
      throw new Error("SecurityError: storage is disabled");
    });

    render(<MobileAppPrompt tenantId={TENANT_A} />);
    expect(
      await screen.findByText("Take the dashboard with you"),
    ).toBeInTheDocument();
  });

  it("still dismisses for the session when localStorage cannot be written", async () => {
    const user = userEvent.setup();
    vi.spyOn(window.localStorage, "setItem").mockImplementation(() => {
      throw new Error("QuotaExceededError");
    });

    render(<MobileAppPrompt tenantId={TENANT_A} />);
    await screen.findByText("Take the dashboard with you");
    await user.click(
      screen.getByRole("button", { name: /dismiss the mobile app suggestion/i }),
    );

    // A failed preference write must not break the dashboard or leave the
    // prompt stuck on screen.
    await waitFor(() =>
      expect(screen.queryByText("Take the dashboard with you")).toBeNull(),
    );
  });

  it("gives the dismiss control an accessible name", async () => {
    render(<MobileAppPrompt tenantId={TENANT_A} />);
    await screen.findByText("Take the dashboard with you");
    const button = screen.getByRole("button", {
      name: /dismiss the mobile app suggestion/i,
    });
    expect(button).toHaveAttribute("type", "button");
  });
});
