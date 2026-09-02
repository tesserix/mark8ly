"use client";

import { useEffect, useState, useTransition } from "react";
import { Button, Badge } from "@tesserix/web";

import type { WebhookDelivery, DeliveryStatus } from "@/lib/api/webhooks";
import {
  listDeliveriesAction,
  replayDeliveryAction,
  testSendWebhookAction,
} from "@/app/(admin)/settings/webhooks/actions";

interface DeliveriesPanelProps {
  webhookId: string;
  editable: boolean;
}

const STATUS_VARIANT: Record<DeliveryStatus, "success" | "error" | "neutral"> = {
  delivered: "success",
  failed: "error",
  pending: "neutral",
};

/**
 * Recent deliveries for one subscription, plus the test-send control.
 * Replay is offered only on `failed` rows — a `pending` delivery is
 * already due to run, and a `delivered` one succeeded, so replaying
 * either would either do nothing or duplicate a webhook the merchant's
 * endpoint already received.
 */
export function DeliveriesPanel({ webhookId, editable }: DeliveriesPanelProps) {
  const [deliveries, setDeliveries] = useState<WebhookDelivery[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loadPending, startLoad] = useTransition();
  const [testPending, startTest] = useTransition();
  const [testResult, setTestResult] = useState<
    { status_code: number; success: boolean; error?: string } | null
  >(null);
  const [replayingId, setReplayingId] = useState<string | null>(null);

  function load() {
    startLoad(async () => {
      const result = await listDeliveriesAction(webhookId);
      if (!result.ok) {
        setError(result.message);
        return;
      }
      setError(null);
      setDeliveries(result.data);
    });
  }

  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => {
    load();
  }, [webhookId]);

  function handleTestSend() {
    setTestResult(null);
    setError(null);
    startTest(async () => {
      const result = await testSendWebhookAction(webhookId);
      if (!result.ok) {
        setError(result.message);
        return;
      }
      setTestResult(result.data);
      load();
    });
  }

  function handleReplay(deliveryId: string) {
    setReplayingId(deliveryId);
    setError(null);
    startLoad(async () => {
      const result = await replayDeliveryAction(webhookId, deliveryId);
      setReplayingId(null);
      if (!result.ok) {
        setError(result.message);
        return;
      }
      load();
    });
  }

  return (
    <div className="space-y-4 border-t border-border-subtle pt-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h4 className="text-sm font-medium text-foreground">Recent deliveries</h4>
        {editable && (
          <Button type="button" variant="outline" size="sm" onClick={handleTestSend} disabled={testPending}>
            {testPending ? "Sending..." : "Send test event"}
          </Button>
        )}
      </div>

      {/* Test-send reports the exact status code the endpoint returned —
          including on failure, since debugging a non-2xx response is the
          whole reason this button exists. */}
      {testResult && (
        <p
          role="status"
          className={`text-sm ${
            testResult.success ? "text-[color:var(--moss-700)]" : "text-[color:var(--danger)]"
          }`}
        >
          Test event responded {testResult.status_code}
          {testResult.error ? ` — ${testResult.error}` : ""}
        </p>
      )}

      {error && (
        <p role="alert" className="text-sm text-[color:var(--danger)]">
          {error}
        </p>
      )}

      {deliveries === null ? (
        <p className="text-sm text-foreground-tertiary">Loading deliveries…</p>
      ) : deliveries.length === 0 ? (
        <p className="text-sm text-foreground-tertiary">No deliveries yet.</p>
      ) : (
        <ul className="space-y-2.5">
          {deliveries.map((d) => (
            <li
              key={d.id}
              className="flex flex-wrap items-center justify-between gap-3 border-b border-border-subtle pb-2.5 text-sm last:border-b-0 last:pb-0"
            >
              <div className="min-w-0">
                <span className="font-medium text-foreground">{d.event_type}</span>
                {d.last_status_code != null && (
                  <span className="ml-2 text-foreground-tertiary">→ {d.last_status_code}</span>
                )}
                {d.last_error && (
                  <p className="mt-0.5 truncate text-xs text-foreground-tertiary">{d.last_error}</p>
                )}
              </div>
              <div className="flex items-center gap-2">
                <Badge variant={STATUS_VARIANT[d.status]}>{d.status}</Badge>
                {editable && d.status === "failed" && (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => handleReplay(d.id)}
                    disabled={replayingId === d.id}
                  >
                    {replayingId === d.id ? "Replaying..." : "Replay"}
                  </Button>
                )}
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
