"use client";

import * as React from "react";
import { useId, useState } from "react";

export interface MediaAltTextInputProps {
  value: string;
  onCommit: (next: string) => void;
  label?: string;
  placeholder?: string;
}

export function MediaAltTextInput({
  value,
  onCommit,
  label = "Alt text",
  placeholder = "Describe the image for screen readers",
}: MediaAltTextInputProps): React.ReactElement {
  const [draft, setDraft] = useState<string>(value);
  const id = useId();

  return (
    <div className="flex flex-col gap-1">
      <label htmlFor={id} className="text-xs uppercase tracking-widest text-[var(--ink-500)]">
        {label}
      </label>
      <input
        id={id}
        type="text"
        value={draft}
        placeholder={placeholder}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={() => {
          if (draft !== value) onCommit(draft);
        }}
        className="rounded-md border border-[var(--ink-200)] bg-[var(--background-elevated)] px-3 py-2 text-sm text-[var(--ink-900)] focus:border-[var(--moss-700)] focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)]"
      />
    </div>
  );
}
