export type JournalUnsubscribeStatus = "idle" | "submitting" | "done" | "error";

interface JournalUnsubscribeFieldsProps {
  status: JournalUnsubscribeStatus;
  errorMessage: string | null;
  onConfirm: () => void;
}

/**
 * Presentational half of JournalUnsubscribeConfirm — pure props in,
 * markup out, no hooks, no fetch. Split out for the same reason
 * JournalSignupFields is: it can be exercised with
 * `renderToStaticMarkup` in tests (see
 * tests/unit/journal-unsubscribe-fields.spec.tsx) without needing a DOM
 * or a click simulator.
 *
 * The button here is the ONLY thing that ever triggers onConfirm — see
 * JournalUnsubscribeConfirm.tsx for why nothing in this feature may ever
 * call the unsubscribe API from a mount/render/effect.
 */
export function JournalUnsubscribeFields({
  status,
  errorMessage,
  onConfirm,
}: JournalUnsubscribeFieldsProps) {
  const disabled = status === "submitting" || status === "done";

  const label =
    status === "submitting"
      ? "Unsubscribing…"
      : status === "done"
        ? "Unsubscribed"
        : status === "error"
          ? "Try again"
          : "Confirm unsubscribe";

  return (
    <div>
      <button
        type="button"
        disabled={disabled}
        onClick={onConfirm}
        className="inline-flex h-11 items-center rounded-md bg-primary px-6 text-sm font-medium text-primary-foreground hover:bg-primary-hover disabled:cursor-not-allowed disabled:bg-ink-600"
      >
        {label}
      </button>
      <div className="mt-4 min-h-[1.125rem]">
        {status === "error" && errorMessage ? (
          <p role="alert" aria-live="polite" className="text-sm text-danger">
            {errorMessage}
          </p>
        ) : null}
        {status === "done" ? (
          <p role="status" aria-live="polite" className="text-sm text-moss-700">
            You&rsquo;ve been unsubscribed. That address is no longer on our
            Journal list.
          </p>
        ) : null}
      </div>
    </div>
  );
}
