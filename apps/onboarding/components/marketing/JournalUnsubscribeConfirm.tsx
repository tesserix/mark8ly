"use client";

// JournalUnsubscribeConfirm — the erasure counterpart to
// JournalSignupForm (#153's email capture). Renders the confirm screen
// for /journal/unsubscribe?token=...
//
// CRITICAL: this component must NEVER submit the unsubscribe request on
// mount, in a useEffect, or from anything other than a direct click on
// the confirm button below. Email clients and security scanners
// routinely prefetch links in the body of an email — including this
// one — before a human ever opens it. If loading this page were enough
// to unsubscribe someone, every prefetch would silently unsubscribe a
// person who never clicked anything. Do NOT "simplify" this into a
// useEffect that fires on mount; that reintroduces exactly this bug.
// See tests/unit/journal-unsubscribe-confirm.spec.tsx, which asserts
// the API is never called just from rendering this component.

import { useState } from "react";

// Relative import (not the usual "@/lib/api/..." alias) so this
// component's own source can be loaded directly by
// tests/unit/journal-unsubscribe-confirm.spec.tsx via loadTsxExport
// (tests/unit/helpers/load-tsx.ts) — that loader's plain `require()` of
// the transpiled output has no tsconfig "paths" resolution, so an
// alias import would fail only inside that test, not in the real build.
import { unsubscribeFromJournal } from "../../lib/api/journal-unsubscribe";

import {
  JournalUnsubscribeFields,
  type JournalUnsubscribeStatus,
} from "./JournalUnsubscribeFields";

interface JournalUnsubscribeConfirmProps {
  /** The `?token=` query param, or null/blank if absent. */
  token: string | null;
}

export function JournalUnsubscribeConfirm({
  token,
}: JournalUnsubscribeConfirmProps) {
  const [status, setStatus] = useState<JournalUnsubscribeStatus>("idle");
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  const trimmedToken = token?.trim() ?? "";

  if (trimmedToken === "") {
    return (
      <p className="text-foreground-secondary">
        This unsubscribe link looks incomplete — it&rsquo;s missing its token.
        If you copied the link by hand, try clicking it directly from the email
        instead. Otherwise, get in touch at{" "}
        <a
          href="/contact"
          className="underline decoration-moss-700 decoration-2 underline-offset-4 hover:text-moss-700"
        >
          /contact
        </a>{" "}
        and we&rsquo;ll remove your address by hand.
      </p>
    );
  }

  async function handleConfirm() {
    // Guarded so a second click while a request is in flight (or after
    // it already succeeded) can't fire a duplicate request — the button
    // is also disabled in those states, but this keeps the handler
    // itself safe regardless of how it's invoked.
    if (status === "submitting" || status === "done") return;

    setStatus("submitting");
    setErrorMessage(null);

    const result = await unsubscribeFromJournal(trimmedToken);

    if (result.ok) {
      setStatus("done");
      return;
    }

    setStatus("error");
    setErrorMessage(result.message);
  }

  return (
    <JournalUnsubscribeFields
      status={status}
      errorMessage={errorMessage}
      onConfirm={() => {
        void handleConfirm();
      }}
    />
  );
}
