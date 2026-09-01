import { Input, Label } from "@tesserix/web";

export type JournalSignupStatus = "idle" | "submitting" | "success" | "error";

interface JournalSignupFieldsProps {
  status: JournalSignupStatus;
  email: string;
  errorMessage: string | null;
  onEmailChange: (value: string) => void;
}

/**
 * Presentational half of JournalSignupForm — pure props in, markup out, no
 * hooks. Split out so the label/aria wiring can be exercised with
 * `renderToStaticMarkup` in tests (see tests/unit/journal-signup-fields.spec.tsx,
 * which loads this file's real JSX via tests/unit/helpers/load-tsx.ts —
 * the same trick tests/unit/mail-link.spec.tsx uses for MailLink, needed
 * because this app's Playwright-based unit tests intercept ordinary JSX
 * with their own pragma).
 */
export function JournalSignupFields({
  status,
  email,
  errorMessage,
  onEmailChange,
}: JournalSignupFieldsProps) {
  const disabled = status === "submitting" || status === "success";
  const hasError = status === "error" && Boolean(errorMessage);

  return (
    <div className="space-y-1.5">
      <Label htmlFor="journal-email" className="text-foreground">
        Email address
      </Label>
      <Input
        id="journal-email"
        type="email"
        inputMode="email"
        autoComplete="email"
        placeholder="you@example.com"
        value={email}
        disabled={disabled}
        required
        aria-invalid={hasError ? true : undefined}
        aria-describedby={hasError ? "journal-email-error" : undefined}
        onChange={(e) => onEmailChange(e.target.value)}
      />
      <div className="min-h-[1.125rem]">
        {hasError ? (
          <p
            id="journal-email-error"
            role="alert"
            aria-live="polite"
            className="text-xs text-danger"
          >
            {errorMessage}
          </p>
        ) : null}
        {status === "success" ? (
          <p role="status" aria-live="polite" className="text-xs text-moss-700">
            You&rsquo;re on the list — we&rsquo;ll email you when the first piece goes up.
          </p>
        ) : null}
      </div>
    </div>
  );
}
