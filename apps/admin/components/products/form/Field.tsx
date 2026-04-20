"use client";

// Field — shared a11y-aware form label wrapper used across the Product
// form tabs. Injects a generated id + aria-invalid + aria-describedby
// onto its single child via cloneElement so native inputs and @tesserix
// wrapper components both get correct screen-reader announcements for
// validation errors and helper text.
//
// Usage:
//   <Field label="Title" error={errors.title?.message} helper="…">
//     <input {...register("title")} />
//   </Field>
//
// The child may already carry an id (takes precedence). Extra props are
// ignored by components that don't forward them, so this works on any
// single child — native or wrapper.

import {
  cloneElement,
  isValidElement,
  useId,
  type ReactElement,
  type ReactNode,
} from "react";

interface FieldChildProps {
  id?: string;
  "aria-invalid"?: boolean;
  "aria-describedby"?: string;
}

export interface FieldProps {
  label: string;
  children: ReactNode;
  error?: string;
  helper?: string;
}

export function Field({ label, children, error, helper }: FieldProps) {
  const generatedId = useId();
  const errorId = `${generatedId}-error`;
  const helperId = `${generatedId}-helper`;

  const enhanced = isValidElement<FieldChildProps>(children)
    ? cloneElement(children as ReactElement<FieldChildProps>, {
        id: children.props.id ?? generatedId,
        "aria-invalid": error ? true : undefined,
        "aria-describedby": error ? errorId : helper ? helperId : undefined,
      })
    : children;

  return (
    <label htmlFor={generatedId} className="flex flex-col gap-2">
      <span className="text-sm font-medium text-[color:var(--ink-900)]">
        {label}
      </span>
      {enhanced}
      {helper && !error && (
        <span
          id={helperId}
          className="text-xs text-[color:var(--ink-900)] opacity-50"
        >
          {helper}
        </span>
      )}
      {error && (
        <span
          id={errorId}
          className="text-xs text-[color:var(--signal,#C23B22)]"
          role="alert"
        >
          {error}
        </span>
      )}
    </label>
  );
}
