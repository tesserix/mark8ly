import type { ReactNode } from "react";

export interface FieldProps {
  /** Unique id for the control — used for aria-describedby wiring */
  id: string;
  /** Visible label */
  label: ReactNode;
  /** Validation error (string or null/undefined). Always renders the
   *  error slot with reserved space so the layout never jitters. */
  error?: string | null | undefined;
  /** Optional muted helper text shown when there is no error */
  hint?: ReactNode;
  /** Helper state — controls hint color (default/success/error) */
  hintState?: "default" | "success" | "error";
  /** The actual control (input, select, textarea, etc.) */
  children: ReactNode;
  /** Optional className passthrough for the wrapping <div> */
  className?: string;
}

/**
 * Field — canonical form-field wrapper used by every Mark8ly form.
 *
 * Renders: Label → control → reserved-space error/hint row.
 *
 * Why the reserved row matters: errors appearing or disappearing
 * mid-interaction is the #1 source of form layout jitter. By always
 * rendering a `min-h-[1.125rem]` slot with either the error, the hint,
 * or nothing, the form cannot shift vertically when validation state
 * changes.
 *
 * Wire-up contract for the consuming control:
 * - Set `id` to the Field's `id`
 * - Set `aria-invalid={error ? true : undefined}`
 * - Set `aria-describedby={error ? `${id}-error` : `${id}-hint`}`
 *
 * The consuming app should import Tailwind classes like `text-foreground`,
 * `text-danger`, `text-moss-700` from its own `mark8ly-tokens.css` import.
 */
export function Field({
  id,
  label,
  error,
  hint,
  hintState = "default",
  children,
  className = "",
}: FieldProps) {
  const hintColor =
    hintState === "success"
      ? "text-moss-700"
      : hintState === "error"
        ? "text-danger"
        : "text-foreground-tertiary";

  return (
    <div className={`space-y-1.5 ${className}`}>
      <label
        htmlFor={id}
        className="block text-sm font-medium text-foreground"
      >
        {label}
      </label>
      {children}
      <div className="min-h-[1.125rem]">
        {error ? (
          <p
            id={`${id}-error`}
            role="alert"
            aria-live="polite"
            className="text-xs text-danger"
          >
            {error}
          </p>
        ) : hint ? (
          <p
            id={`${id}-hint`}
            aria-live="polite"
            className={`text-xs ${hintColor}`}
          >
            {hint}
          </p>
        ) : null}
      </div>
    </div>
  );
}
