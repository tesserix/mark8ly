"use client";

// JournalSignupForm — #153: "Notify me when the first piece goes up" on
// the /blog "coming soon" page.
//
// Posts to this app's own /api/journal-subscribe route (never
// marketplace-api directly — see lib/api/journal-signup.ts). Idempotent
// resubmission and rate limiting are enforced server-side; this component
// only needs to render the four states plainly: idle, submitting,
// success, error.

import { useState } from "react";

import { subscribeToJournal } from "@/lib/api/journal-signup";

import { JournalSignupFields, type JournalSignupStatus } from "./JournalSignupFields";

export function JournalSignupForm() {
  const [email, setEmail] = useState("");
  const [status, setStatus] = useState<JournalSignupStatus>("idle");
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  const disabled = status === "submitting" || status === "success";

  async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (disabled) return;

    setStatus("submitting");
    setErrorMessage(null);

    const result = await subscribeToJournal(email);

    if (result.ok) {
      setStatus("success");
      return;
    }

    setStatus("error");
    setErrorMessage(result.message);
  }

  return (
    <form
      onSubmit={(e) => {
        void handleSubmit(e);
      }}
      noValidate
      className="mt-10 max-w-sm border-t border-border-subtle pt-8"
    >
      <p className="mb-4 text-sm font-medium text-foreground">
        Notify me when the first piece goes up
      </p>
      <JournalSignupFields
        status={status}
        email={email}
        errorMessage={errorMessage}
        onEmailChange={setEmail}
      />
      <button
        type="submit"
        disabled={disabled}
        className="mt-4 inline-flex h-11 items-center rounded-md bg-primary px-6 text-sm font-medium text-primary-foreground hover:bg-primary-hover disabled:cursor-not-allowed disabled:bg-ink-600"
      >
        {status === "submitting"
          ? "Sending…"
          : status === "success"
            ? "Subscribed"
            : "Notify me"}
      </button>
    </form>
  );
}
