"use client";

// WebhooksSettingsClient — outbound webhook subscriptions (#562 task 9).
//
// List, create, edit, enable/disable, delete, test-send, and browse
// deliveries for a store's webhook subscriptions. Follows the same shape
// as WarehousesSettingsClient: an inline create/edit form swaps in for the
// list, an AlertDialog confirms delete, and mutations go through the
// server actions in app/(admin)/settings/webhooks/actions.ts.
//
// The one thing this surface has that no other settings page does: the
// signing secret. It exists on the wire exactly once, in the response to
// Create, and WebhookSecretDialog is the only place in this component
// tree that ever touches it — it's never stored in this component's own
// state beyond the dialog's lifetime, and never logged.

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { Button, AlertDialog } from "@tesserix/web";

import type { WebhookSubscription } from "@/lib/api/webhooks";
import {
  createWebhookAction,
  patchWebhookAction,
  deleteWebhookAction,
} from "@/app/(admin)/settings/webhooks/actions";
import { WebhookForm } from "./webhooks/WebhookForm";
import { WebhookRow } from "./webhooks/WebhookRow";
import { WebhookSecretDialog } from "./webhooks/WebhookSecretDialog";

interface WebhooksSettingsClientProps {
  webhooks: WebhookSubscription[];
  editable: boolean;
}

type Editing = { mode: "none" } | { mode: "new" } | { mode: "edit"; id: string };

export function WebhooksSettingsClient({ webhooks, editable }: WebhooksSettingsClientProps) {
  const router = useRouter();
  const [editing, setEditing] = useState<Editing>({ mode: "none" });
  const [confirmDelete, setConfirmDelete] = useState<WebhookSubscription | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();
  const [revealedSecret, setRevealedSecret] = useState<string | null>(null);

  function handleCreate(input: { url: string; event_types: string[] }) {
    return new Promise<{ ok: boolean; message?: string; field?: string }>((resolve) => {
      startTransition(async () => {
        const result = await createWebhookAction(input);
        if (!result.ok) {
          resolve({ ok: false, message: result.message, field: result.field });
          return;
        }
        setEditing({ mode: "none" });
        // Reveal the secret before refreshing the list — it exists only
        // on this response and nowhere else, ever.
        setRevealedSecret(result.data.secret);
        router.refresh();
        resolve({ ok: true });
      });
    });
  }

  function handleUpdate(id: string, input: { url: string; event_types: string[] }) {
    return new Promise<{ ok: boolean; message?: string; field?: string }>((resolve) => {
      startTransition(async () => {
        const result = await patchWebhookAction(id, input);
        if (!result.ok) {
          resolve({ ok: false, message: result.message, field: result.field });
          return;
        }
        setEditing({ mode: "none" });
        router.refresh();
        resolve({ ok: true });
      });
    });
  }

  function handleToggleEnabled(webhook: WebhookSubscription, enabled: boolean) {
    setError(null);
    startTransition(async () => {
      const result = await patchWebhookAction(webhook.id, { enabled });
      if (!result.ok) {
        setError(result.message);
        return;
      }
      router.refresh();
    });
  }

  function handleDeleteConfirmed() {
    const target = confirmDelete;
    if (!target) return;
    setConfirmDelete(null);
    setError(null);
    startTransition(async () => {
      const result = await deleteWebhookAction(target.id);
      if (!result.ok) {
        setError(result.message);
        return;
      }
      router.refresh();
    });
  }

  if (editing.mode === "new") {
    return (
      <section className="border-t border-border-subtle pt-8">
        <h2 className="mb-6 font-serif text-xl text-[color:var(--ink-900)]">
          Add a webhook
        </h2>
        <WebhookForm
          onSubmit={handleCreate}
          onCancel={() => setEditing({ mode: "none" })}
          submitLabel="Create webhook"
        />
      </section>
    );
  }

  if (editing.mode === "edit") {
    const target = webhooks.find((w) => w.id === editing.id);
    if (target) {
      return (
        <section className="border-t border-border-subtle pt-8">
          <h2 className="mb-6 font-serif text-xl text-[color:var(--ink-900)]">
            Edit webhook
          </h2>
          <WebhookForm
            existing={target}
            onSubmit={(input) => handleUpdate(target.id, input)}
            onCancel={() => setEditing({ mode: "none" })}
            submitLabel="Save changes"
          />
        </section>
      );
    }
  }

  return (
    <section>
      {error && (
        <p role="alert" className="mb-4 rounded-md border border-[color:var(--danger)]/25 bg-[color:var(--danger)]/[0.06] px-4 py-2.5 text-sm text-[color:var(--danger)]">
          {error}
        </p>
      )}

      {webhooks.length === 0 ? (
        <div className="border-t border-border-subtle py-10">
          <p className="text-sm text-foreground-tertiary">
            No webhooks yet. Add one to start sending order, return, product,
            and category events to your own endpoint.
          </p>
          {editable && (
            <Button type="button" className="mt-4" onClick={() => setEditing({ mode: "new" })}>
              Add webhook
            </Button>
          )}
        </div>
      ) : (
        <>
          <ul className="mt-6 border-t border-border-subtle">
            {webhooks.map((w) => (
              <WebhookRow
                key={w.id}
                webhook={w}
                editable={editable}
                pending={pending}
                onEdit={() => setEditing({ mode: "edit", id: w.id })}
                onDelete={() => setConfirmDelete(w)}
                onToggleEnabled={(enabled) => handleToggleEnabled(w, enabled)}
              />
            ))}
          </ul>

          {editable && (
            <Button type="button" className="mt-6" onClick={() => setEditing({ mode: "new" })} disabled={pending}>
              Add webhook
            </Button>
          )}
        </>
      )}

      <AlertDialog
        isOpen={confirmDelete !== null}
        onClose={() => setConfirmDelete(null)}
        title={confirmDelete ? "Delete this webhook?" : ""}
        message={
          confirmDelete
            ? "This stops all future deliveries to this endpoint immediately and cannot be undone. If you need it again, you'll create a new subscription with a new secret."
            : ""
        }
        type="confirm"
        confirmLabel="Delete"
        cancelLabel="Cancel"
        onConfirm={handleDeleteConfirmed}
        onCancel={() => setConfirmDelete(null)}
      />

      <WebhookSecretDialog secret={revealedSecret} onDismiss={() => setRevealedSecret(null)} />
    </section>
  );
}
