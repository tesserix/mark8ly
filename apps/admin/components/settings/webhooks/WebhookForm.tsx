"use client";

import { useState, useTransition } from "react";
import { Button, Input } from "@tesserix/web";

import type { WebhookSubscription } from "@/lib/api/webhooks";
import { EventTypeCheckboxGroup } from "./EventTypeCheckboxGroup";

export interface WebhookFormSubmitResult {
  ok: boolean;
  message?: string;
  field?: string;
}

interface WebhookFormProps {
  existing?: WebhookSubscription;
  onSubmit: (input: { url: string; event_types: string[] }) => Promise<WebhookFormSubmitResult>;
  onCancel: () => void;
  submitLabel: string;
}

/**
 * Create/edit form shared by both flows. The backend maps SSRF failures
 * (not-HTTPS, private address, unresolvable, malformed, too long) to
 * distinct messages via mapSSRFErr — this surfaces whatever message comes
 * back verbatim, wired to the URL field with aria-describedby, rather than
 * collapsing every case into a generic "invalid URL".
 */
export function WebhookForm({ existing, onSubmit, onCancel, submitLabel }: WebhookFormProps) {
  const [url, setUrl] = useState(existing?.url ?? "");
  const [eventTypes, setEventTypes] = useState<string[]>(existing?.event_types ?? []);
  const [error, setError] = useState<{ message: string; field?: string } | null>(null);
  const [pending, startTransition] = useTransition();

  const urlErrorId = "webhook-url-error";
  const eventTypesErrorId = "webhook-event-types-error";

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    if (eventTypes.length === 0) {
      setError({ message: "Select at least one event type.", field: "event_types" });
      return;
    }
    startTransition(async () => {
      const result = await onSubmit({ url: url.trim(), event_types: eventTypes });
      if (!result.ok) {
        setError({ message: result.message ?? "Something went wrong.", field: result.field });
      }
    });
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      <div className="space-y-1.5">
        <label htmlFor="webhook-url" className="block text-sm font-medium text-foreground">
          Endpoint URL
        </label>
        <Input
          id="webhook-url"
          type="url"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          placeholder="https://example.com/webhooks/mark8ly"
          disabled={pending}
          required
          aria-invalid={error?.field === "url" ? true : undefined}
          aria-describedby={error?.field === "url" ? urlErrorId : undefined}
        />
        <p className="text-xs text-foreground-tertiary">
          Must be HTTPS and publicly resolvable. We sign every request with
          your subscription's secret.
        </p>
        {error?.field === "url" && (
          <p id={urlErrorId} role="alert" className="text-sm text-[color:var(--danger)]">
            {error.message}
          </p>
        )}
      </div>

      <div>
        <EventTypeCheckboxGroup
          value={eventTypes}
          onChange={setEventTypes}
          disabled={pending}
          describedBy={error?.field === "event_types" ? eventTypesErrorId : undefined}
        />
        {error?.field === "event_types" && (
          <p id={eventTypesErrorId} role="alert" className="mt-2 text-sm text-[color:var(--danger)]">
            {error.message}
          </p>
        )}
      </div>

      {error && !error.field && (
        <p role="alert" className="text-sm text-[color:var(--danger)]">
          {error.message}
        </p>
      )}

      <div className="flex items-center gap-3">
        <Button type="submit" disabled={pending}>
          {pending ? "Saving..." : submitLabel}
        </Button>
        <Button type="button" variant="ghost" onClick={onCancel} disabled={pending}>
          Cancel
        </Button>
      </div>
    </form>
  );
}
