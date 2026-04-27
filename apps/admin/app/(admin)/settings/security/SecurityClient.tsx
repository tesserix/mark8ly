"use client";

import { useState, useTransition } from "react";
import {
  LinkedProvidersPanel,
  type LinkedProvider,
} from "@repo/ui/auth/linked-providers-panel";
import { LinkProviderPrompt } from "@repo/ui/auth/link-provider-prompt";
import { getGoogleCredential } from "@/lib/gip/google-gsi";
import { signInWithGoogle, GIPError } from "@/lib/gip/signup";
import { linkGoogleToInternalPassword } from "@/lib/gip/link";
import { refreshProvidersAction } from "./actions";

interface SecurityClientProps {
  initialProviders: LinkedProvider[];
  loadError: string | null;
}

export function SecurityClient({
  initialProviders,
  loadError,
}: SecurityClientProps) {
  const [providers, setProviders] =
    useState<LinkedProvider[]>(initialProviders);
  const [error, setError] = useState<string | null>(loadError);
  const [pending, startTransition] = useTransition();
  const [needConfirmation, setNeedConfirmation] = useState<{
    email: string;
    pendingIdpCredential: string;
  } | null>(null);
  const [linkPromptError, setLinkPromptError] = useState<string | null>(null);

  function refresh() {
    startTransition(async () => {
      try {
        const next = await refreshProvidersAction();
        setProviders(next);
        setError(null);
      } catch (err) {
        setError(
          err instanceof Error
            ? err.message
            : "Could not refresh providers.",
        );
      }
    });
  }

  async function handleLinkGoogle() {
    setError(null);
    try {
      const { credential } = await getGoogleCredential();
      const result = await signInWithGoogle(credential);
      if (result.kind === "needConfirmation") {
        setNeedConfirmation({
          email: result.email,
          pendingIdpCredential: result.pendingIdpCredential,
        });
        return;
      }
      // Direct success — provider linked at GIP. Refresh the list.
      refresh();
    } catch (err) {
      setError(
        err instanceof Error
          ? `Could not link Google: ${err.message}`
          : "Could not link Google.",
      );
    }
  }

  async function handleLinkConfirm(password: string) {
    if (!needConfirmation) return;
    setLinkPromptError(null);
    try {
      await linkGoogleToInternalPassword(
        needConfirmation.email,
        password,
        needConfirmation.pendingIdpCredential,
      );
      setNeedConfirmation(null);
      refresh();
    } catch (err) {
      if (err instanceof GIPError && err.code === "invalid_credentials") {
        setLinkPromptError("That password is incorrect. Please try again.");
        return;
      }
      setLinkPromptError(
        err instanceof Error ? err.message : "Could not link Google.",
      );
    }
  }

  function handleLinkCancel() {
    setNeedConfirmation(null);
    setLinkPromptError(null);
  }

  async function handleUnlink(_providerId: string): Promise<void> {
    // v1: unlink not yet implemented. Surface a friendly error.
    setError(
      "Removing a sign-in method requires a verification step we haven't built yet — please contact support.",
    );
    throw new Error("unlink_unavailable");
  }

  return (
    <div className="space-y-4">
      <LinkedProvidersPanel
        providers={providers}
        onLinkGoogle={handleLinkGoogle}
        onUnlink={handleUnlink}
        variant="admin"
        pending={pending}
      />
      {error && (
        <p role="alert" className="text-sm text-danger">
          {error}
        </p>
      )}
      {needConfirmation && (
        <LinkProviderPrompt
          email={needConfirmation.email}
          variant="admin"
          error={linkPromptError}
          onConfirm={handleLinkConfirm}
          onCancel={handleLinkCancel}
        />
      )}
    </div>
  );
}
