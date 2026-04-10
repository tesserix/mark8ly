"use client";

import { useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogClose,
} from "@tesserix/web";
import type { AdminCampaign, SessionHeaders } from "@/lib/api/campaigns-api";
import {
  sendCampaign,
  pauseCampaign,
  resumeCampaign,
  deleteCampaign,
} from "@/lib/api/campaigns-api";

interface CampaignActionsProps {
  campaign: AdminCampaign;
  storeId: string;
  session: SessionHeaders;
}

export function CampaignActions({
  campaign,
  storeId,
  session,
}: CampaignActionsProps) {
  const router = useRouter();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showSendDialog, setShowSendDialog] = useState(false);
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);

  const handleAction = useCallback(
    async (action: () => Promise<unknown>, successMessage?: string) => {
      setLoading(true);
      setError(null);
      try {
        const result = await action();
        if (!result) {
          setError("Action failed. Please try again.");
          setLoading(false);
          return;
        }
        router.refresh();
      } catch {
        setError("An unexpected error occurred.");
      } finally {
        setLoading(false);
      }
    },
    [router],
  );

  const c = campaign;

  return (
    <div className="flex items-center gap-3">
      {error && (
        <p className="text-xs text-signal-700">{error}</p>
      )}

      {/* Send — available for draft campaigns */}
      {c.status === "draft" && (
        <button
          onClick={() => setShowSendDialog(true)}
          disabled={loading}
          className="inline-flex items-center gap-2 rounded-md bg-ink-900 px-4 py-2 text-sm font-medium text-paper-200 transition hover:bg-ink-800 disabled:opacity-50"
        >
          Send
        </button>
      )}

      {/* Pause — available for sending campaigns */}
      {c.status === "sending" && (
        <button
          onClick={() =>
            handleAction(() => pauseCampaign(storeId, c.id, session))
          }
          disabled={loading}
          className="inline-flex items-center gap-2 rounded-md border border-ink-200 bg-white px-4 py-2 text-sm font-medium text-ink-700 transition hover:bg-ink-50 disabled:opacity-50"
        >
          {loading ? "Pausing..." : "Pause"}
        </button>
      )}

      {/* Resume — available for paused campaigns */}
      {c.status === "paused" && (
        <button
          onClick={() =>
            handleAction(() => resumeCampaign(storeId, c.id, session))
          }
          disabled={loading}
          className="inline-flex items-center gap-2 rounded-md bg-moss-700 px-4 py-2 text-sm font-medium text-white transition hover:bg-moss-800 disabled:opacity-50"
        >
          {loading ? "Resuming..." : "Resume"}
        </button>
      )}

      {/* Delete — available for draft campaigns only */}
      {c.status === "draft" && (
        <button
          onClick={() => setShowDeleteDialog(true)}
          disabled={loading}
          className="inline-flex items-center gap-2 rounded-md border border-signal-200 bg-white px-4 py-2 text-sm font-medium text-signal-700 transition hover:bg-signal-50 disabled:opacity-50"
        >
          Delete
        </button>
      )}

      {/* Send confirmation dialog */}
      <Dialog open={showSendDialog} onOpenChange={setShowSendDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Send campaign?</DialogTitle>
            <DialogDescription>
              This will send &ldquo;{c.name}&rdquo; to{" "}
              {c.total_recipients > 0
                ? `${c.total_recipients.toLocaleString()} recipients`
                : "your audience"}
              . This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <DialogClose asChild>
              <button className="inline-flex items-center gap-2 rounded-md border border-ink-200 bg-white px-4 py-2 text-sm font-medium text-ink-700 transition hover:bg-ink-50">
                Cancel
              </button>
            </DialogClose>
            <button
              onClick={() => {
                setShowSendDialog(false);
                handleAction(() => sendCampaign(storeId, c.id, session));
              }}
              disabled={loading}
              className="inline-flex items-center gap-2 rounded-md bg-ink-900 px-4 py-2 text-sm font-medium text-paper-200 transition hover:bg-ink-800 disabled:opacity-50"
            >
              {loading ? "Sending..." : "Send now"}
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete confirmation dialog */}
      <Dialog open={showDeleteDialog} onOpenChange={setShowDeleteDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete campaign?</DialogTitle>
            <DialogDescription>
              This will permanently delete &ldquo;{c.name}&rdquo;. This action
              cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <DialogClose asChild>
              <button className="inline-flex items-center gap-2 rounded-md border border-ink-200 bg-white px-4 py-2 text-sm font-medium text-ink-700 transition hover:bg-ink-50">
                Cancel
              </button>
            </DialogClose>
            <button
              onClick={async () => {
                setShowDeleteDialog(false);
                setLoading(true);
                const ok = await deleteCampaign(storeId, c.id, session);
                if (ok) {
                  router.push("/marketing/campaigns");
                  router.refresh();
                } else {
                  setError("Failed to delete campaign.");
                }
                setLoading(false);
              }}
              disabled={loading}
              className="inline-flex items-center gap-2 rounded-md bg-signal-700 px-4 py-2 text-sm font-medium text-white transition hover:bg-signal-800 disabled:opacity-50"
            >
              {loading ? "Deleting..." : "Delete"}
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
