"use client";

import { useState } from "react";
import { Button } from "@tesserix/web";
import { ChevronDown, ChevronUp } from "lucide-react";

import type { WebhookSubscription } from "@/lib/api/webhooks";
import { ToggleSwitch } from "@/components/settings/BrandingSettingsClient";
import { eventTypeLabel } from "./eventTypes";
import { DeliveriesPanel } from "./DeliveriesPanel";

interface WebhookRowProps {
  webhook: WebhookSubscription;
  editable: boolean;
  pending: boolean;
  onEdit: () => void;
  onDelete: () => void;
  onToggleEnabled: (enabled: boolean) => void;
}

export function WebhookRow({
  webhook,
  editable,
  pending,
  onEdit,
  onDelete,
  onToggleEnabled,
}: WebhookRowProps) {
  const [expanded, setExpanded] = useState(false);

  return (
    <li className="border-b border-border-subtle py-5">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0 space-y-1.5">
          <p className="break-all font-mono text-sm text-foreground">{webhook.url}</p>
          <p className="text-xs text-foreground-tertiary">
            {webhook.event_types.length === 1
              ? eventTypeLabel(webhook.event_types[0] as string)
              : `${webhook.event_types.length} event types`}
          </p>

          {/* A disabled subscription must show WHY, prominently and
              without opening a delivery — the backend's disabled_reason
              is written in plain language for exactly this. */}
          {!webhook.enabled && webhook.disabled_reason && (
            <p
              role="status"
              className="mt-2 max-w-prose rounded-md border border-[color:var(--danger)]/25 bg-[color:var(--danger)]/[0.06] px-3 py-2 text-sm text-[color:var(--danger)]"
            >
              Disabled: {webhook.disabled_reason}
            </p>
          )}
        </div>

        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => setExpanded((v) => !v)}
            aria-expanded={expanded}
            className="inline-flex items-center gap-1 rounded-md px-2 py-1.5 text-sm text-foreground-secondary transition-colors hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
          >
            Deliveries
            {expanded ? (
              <ChevronUp className="h-3.5 w-3.5" aria-hidden="true" />
            ) : (
              <ChevronDown className="h-3.5 w-3.5" aria-hidden="true" />
            )}
          </button>

          {editable && (
            <>
              <ToggleSwitch
                checked={webhook.enabled}
                onChange={onToggleEnabled}
                disabled={pending}
              />
              <Button type="button" variant="outline" size="sm" onClick={onEdit} disabled={pending}>
                Edit
              </Button>
              <Button
                type="button"
                variant="destructive"
                size="sm"
                onClick={onDelete}
                disabled={pending}
              >
                Delete
              </Button>
            </>
          )}
        </div>
      </div>

      {expanded && (
        <div className="mt-4">
          <DeliveriesPanel webhookId={webhook.id} editable={editable} />
        </div>
      )}
    </li>
  );
}
