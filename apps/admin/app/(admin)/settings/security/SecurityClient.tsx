"use client";

import { useState } from "react";
import {
  LinkedProvidersPanel,
  type LinkedProvider,
} from "@repo/ui/auth/linked-providers-panel";

interface SecurityClientProps {
  initialProviders: LinkedProvider[];
  loadError: string | null;
}

export function SecurityClient({
  initialProviders,
  loadError,
}: SecurityClientProps) {
  const [providers] = useState<LinkedProvider[]>(initialProviders);
  const [error, setError] = useState<string | null>(loadError);

  // Linking a provider from this panel was a GIP-only flow (a GSI popup
  // exchanged at Identity Toolkit). It has no Zitadel equivalent yet:
  // Zitadel links an IDP through an auth-request-scoped intent, which
  // this settings page cannot mint. Surface that plainly rather than
  // pretending to start a flow.
  async function handleLinkGoogle() {
    setError(
      "Adding a Google sign-in method from settings isn't available yet — sign in with Google from the sign-in screen instead.",
    );
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
      />
      {error && (
        <p role="alert" className="text-sm text-danger">
          {error}
        </p>
      )}
    </div>
  );
}
