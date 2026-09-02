"use client";

import { useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  Button,
  Checkbox,
} from "@tesserix/web";
import { Check, Copy } from "lucide-react";

interface WebhookSecretDialogProps {
  /** null hides the dialog. Pass the freshly-created secret to show it. */
  secret: string | null;
  onDismiss: () => void;
}

/**
 * The signing secret is shown here exactly once — the create response
 * carries it and no later endpoint ever returns it again. Every affordance
 * in this dialog exists to stop a merchant from losing it by accident:
 *
 *   - Escape and outside-click are disabled (`dismissOnOutsideClick`,
 *     no close button) — the only exit is the explicit "Done" button.
 *   - "Done" is disabled until the merchant checks a box acknowledging
 *     they've copied it, so the consequence of closing without copying
 *     (delete + recreate is the only recovery) is unmissable *before*
 *     they dismiss it, not something they discover on the next visit.
 */
export function WebhookSecretDialog({ secret, onDismiss }: WebhookSecretDialogProps) {
  const [copied, setCopied] = useState(false);
  const [acknowledged, setAcknowledged] = useState(false);

  if (!secret) return null;

  async function handleCopy() {
    try {
      await navigator.clipboard.writeText(secret as string);
      setCopied(true);
    } catch {
      // Clipboard API can be unavailable (older browser, insecure
      // context). The secret is still visible and selectable as text, so
      // this isn't fatal — just leave the button in its un-copied state.
    }
  }

  function handleDone() {
    setCopied(false);
    setAcknowledged(false);
    onDismiss();
  }

  return (
    <Dialog open onOpenChange={() => {}}>
      <DialogContent showCloseButton={false} dismissOnOutsideClick={false}>
        <DialogHeader>
          <DialogTitle>Save your signing secret</DialogTitle>
          <DialogDescription>
            This is the only time Mark8ly will show you this secret. Copy it
            now and store it somewhere safe. If you lose it, the only way to
            get a new one is to delete this webhook and create another.
          </DialogDescription>
        </DialogHeader>

        <div className="flex items-center gap-2 rounded-md border border-border bg-[color:var(--background-elevated)] px-3 py-2.5">
          <code className="flex-1 overflow-x-auto whitespace-nowrap font-mono text-sm text-foreground">
            {secret}
          </code>
          <Button type="button" variant="outline" size="sm" onClick={handleCopy}>
            {copied ? (
              <>
                <Check className="h-3.5 w-3.5" aria-hidden="true" />
                Copied
              </>
            ) : (
              <>
                <Copy className="h-3.5 w-3.5" aria-hidden="true" />
                Copy
              </>
            )}
          </Button>
        </div>

        <label className="flex items-start gap-2.5 pt-2 text-sm text-foreground-secondary">
          <Checkbox
            checked={acknowledged}
            onCheckedChange={(checked) => setAcknowledged(checked === true)}
            className="mt-0.5"
          />
          I have copied and saved this secret. I understand Mark8ly will not
          show it to me again.
        </label>

        <DialogFooter>
          <Button type="button" onClick={handleDone} disabled={!acknowledged}>
            Done
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
