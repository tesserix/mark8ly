/**
 * WebhooksSettingsClient unit tests — Vitest + Testing Library.
 *
 * Covers the requirements from #562 task 9 that are easy to silently
 * regress:
 *   1. The secret is shown once after creation, with a copy control and
 *      an explicit warning that it will not be shown again.
 *   2. A disabled subscription renders its disabled_reason prominently,
 *      without having to open a delivery.
 *   3. The delivery list shows status + response code, and offers replay
 *      only on failed deliveries.
 *   4. Test-send reports the status code it got back, including failure.
 *   5. API validation errors (SSRF messages) surface readably, wired via
 *      aria-describedby.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import type { WebhookSubscription, WebhookDelivery } from "@/lib/api/webhooks";

const createWebhookAction = vi.fn();
const patchWebhookAction = vi.fn();
const deleteWebhookAction = vi.fn();
const testSendWebhookAction = vi.fn();
const listDeliveriesAction = vi.fn();
const replayDeliveryAction = vi.fn();
const refresh = vi.fn();

vi.mock("@/app/(admin)/settings/webhooks/actions", () => ({
  createWebhookAction: (...args: unknown[]) => createWebhookAction(...args),
  patchWebhookAction: (...args: unknown[]) => patchWebhookAction(...args),
  deleteWebhookAction: (...args: unknown[]) => deleteWebhookAction(...args),
  testSendWebhookAction: (...args: unknown[]) => testSendWebhookAction(...args),
  listDeliveriesAction: (...args: unknown[]) => listDeliveriesAction(...args),
  replayDeliveryAction: (...args: unknown[]) => replayDeliveryAction(...args),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh }),
}));

import { WebhooksSettingsClient } from "@/components/settings/WebhooksSettingsClient";

function webhook(overrides: Partial<WebhookSubscription> = {}): WebhookSubscription {
  return {
    id: "wh-1",
    url: "https://example.com/webhooks/mark8ly",
    event_types: ["order.placed"],
    enabled: true,
    disabled_reason: null,
    disabled_at: null,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

function delivery(overrides: Partial<WebhookDelivery> = {}): WebhookDelivery {
  return {
    id: "del-1",
    event_type: "order.placed",
    aggregate_id: "order-1",
    status: "delivered",
    attempts: 1,
    next_attempt_at: "2026-01-01T00:00:00Z",
    last_status_code: 200,
    last_error: null,
    delivered_at: "2026-01-01T00:00:05Z",
    created_at: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

// jsdom has neither ResizeObserver (Radix's Checkbox/Switch use it to size
// themselves) nor a writable navigator.clipboard — polyfill both so the
// component tree the real app renders can mount here too.
class FakeResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}

const writeText = vi.fn().mockResolvedValue(undefined);

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal("ResizeObserver", FakeResizeObserver);
});

// Defining navigator.clipboard must happen AFTER userEvent.setup() runs —
// user-event installs its own clipboard stub for copy/paste emulation the
// first time setup() is called, which would otherwise clobber ours.
function stubClipboard() {
  Object.defineProperty(navigator, "clipboard", {
    value: { writeText },
    configurable: true,
  });
}

describe("WebhooksSettingsClient — secret reveal", () => {
  it("shows the secret exactly once after creation, with a copy control and a permanent warning", async () => {
    const user = userEvent.setup();
    stubClipboard();
    createWebhookAction.mockResolvedValue({
      ok: true,
      data: {
        subscription: webhook(),
        secret: "whsec_only_shown_once",
      },
    });

    render(<WebhooksSettingsClient webhooks={[]} editable />);

    await user.click(screen.getByRole("button", { name: "Add webhook" }));
    await user.type(screen.getByLabelText("Endpoint URL"), "https://example.com/hook");
    await user.click(screen.getByRole("checkbox", { name: "Order placed" }));
    await user.click(screen.getByRole("button", { name: "Create webhook" }));

    // The secret itself is shown, plainly, in the dialog.
    expect(await screen.findByText("whsec_only_shown_once")).toBeInTheDocument();
    // A copy control exists.
    const copyButton = screen.getByRole("button", { name: /copy/i });

    // The unmissable warning it will never be shown again.
    expect(
      screen.getByText(/only time mark8ly will show you this secret/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/delete this webhook and create another/i),
    ).toBeInTheDocument();

    // The Done button is gated on acknowledging the secret was copied —
    // dismissing without acknowledging is not allowed, since the only
    // recovery afterwards is delete + recreate.
    const doneButton = screen.getByRole("button", { name: "Done" });
    expect(doneButton).toBeDisabled();

    await user.click(copyButton);
    await waitFor(() => expect(copyButton).toHaveTextContent("Copied"));
    expect(writeText).toHaveBeenCalledWith("whsec_only_shown_once");

    await user.click(
      screen.getByRole("checkbox", {
        name: /i have copied and saved this secret/i,
      }),
    );
    expect(doneButton).toBeEnabled();

    await user.click(doneButton);
    await waitFor(() =>
      expect(screen.queryByText("whsec_only_shown_once")).not.toBeInTheDocument(),
    );
  });
});

describe("WebhooksSettingsClient — disabled subscriptions", () => {
  it("renders disabled_reason prominently without opening a delivery", () => {
    render(
      <WebhooksSettingsClient
        webhooks={[
          webhook({
            enabled: false,
            disabled_reason:
              "disabled automatically after 10 consecutive delivery failures — fix the endpoint and re-enable",
          }),
        ]}
        editable
      />,
    );

    expect(
      screen.getByText(
        /disabled automatically after 10 consecutive delivery failures/i,
      ),
    ).toBeInTheDocument();
  });

  it("does not show a disabled reason banner for an enabled subscription", () => {
    render(<WebhooksSettingsClient webhooks={[webhook({ enabled: true })]} editable />);
    expect(screen.queryByText(/disabled/i)).not.toBeInTheDocument();
  });
});

describe("WebhooksSettingsClient — deliveries", () => {
  it("shows status and response code, and offers replay only on failed deliveries", async () => {
    const user = userEvent.setup();
    listDeliveriesAction.mockResolvedValue({
      ok: true,
      data: [
        delivery({ id: "d-delivered", status: "delivered", last_status_code: 200 }),
        delivery({ id: "d-failed", status: "failed", last_status_code: 500 }),
        delivery({ id: "d-pending", status: "pending", last_status_code: null }),
      ],
    });

    render(<WebhooksSettingsClient webhooks={[webhook()]} editable />);
    await user.click(screen.getByRole("button", { name: /deliveries/i }));

    const delivered = (await screen.findByText("delivered")).closest("li") as HTMLElement;
    expect(within(delivered).getByText(/200/)).toBeInTheDocument();
    expect(within(delivered).queryByRole("button", { name: "Replay" })).not.toBeInTheDocument();

    const failed = screen.getByText("failed").closest("li") as HTMLElement;
    expect(within(failed).getByText(/500/)).toBeInTheDocument();
    expect(within(failed).getByRole("button", { name: "Replay" })).toBeInTheDocument();

    const pending = screen.getByText("pending").closest("li") as HTMLElement;
    expect(within(pending).queryByRole("button", { name: "Replay" })).not.toBeInTheDocument();
  });

  it("test-send reports the status code it got back, including on failure", async () => {
    const user = userEvent.setup();
    listDeliveriesAction.mockResolvedValue({ ok: true, data: [] });
    testSendWebhookAction.mockResolvedValue({
      ok: true,
      data: { status_code: 403, success: false, error: "endpoint returned 403" },
    });

    render(<WebhooksSettingsClient webhooks={[webhook()]} editable />);
    await user.click(screen.getByRole("button", { name: /deliveries/i }));
    await user.click(await screen.findByRole("button", { name: /send test event/i }));

    const status = await screen.findByRole("status");
    expect(status).toHaveTextContent("403");
  });
});

describe("WebhooksSettingsClient — form validation errors", () => {
  it("surfaces API validation errors readably, wired with aria-describedby", async () => {
    const user = userEvent.setup();
    stubClipboard();
    createWebhookAction.mockResolvedValue({
      ok: false,
      code: "validation_failed",
      message: "webhook url resolves to a private or otherwise non-public address",
      field: "url",
    });

    render(<WebhooksSettingsClient webhooks={[]} editable />);

    await user.click(screen.getByRole("button", { name: "Add webhook" }));
    const urlInput = screen.getByLabelText("Endpoint URL");
    await user.type(urlInput, "https://10.0.0.5/hook");
    await user.click(screen.getByRole("checkbox", { name: "Order placed" }));
    await user.click(screen.getByRole("button", { name: "Create webhook" }));

    const errorText = await screen.findByText(
      /resolves to a private or otherwise non-public address/i,
    );
    expect(errorText).toHaveAttribute("id", urlInput.getAttribute("aria-describedby"));
    expect(urlInput).toHaveAttribute("aria-invalid", "true");
  });
});
