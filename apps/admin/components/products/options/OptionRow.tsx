"use client";

import * as React from "react";
import { Trash2, X } from "lucide-react";
import type { OptionDraft } from "./OptionsEditor";

export interface OptionRowProps {
  option: OptionDraft;
  onNameChange: (next: string) => void;
  onValuesChange: (next: OptionDraft["values"]) => void;
  onRemove: () => void;
}

export function OptionRow({
  option,
  onNameChange,
  onValuesChange,
  onRemove,
}: OptionRowProps): React.ReactElement {
  const [inflight, setInflight] = React.useState("");

  const commitValue = (): void => {
    const trimmed = inflight.trim();
    if (trimmed.length === 0) {
      return;
    }
    onValuesChange([...option.values, { value: trimmed }]);
    setInflight("");
  };

  const removeValue = (index: number): void => {
    onValuesChange(option.values.filter((_, i) => i !== index));
  };

  const fieldClass =
    "h-10 w-full rounded-md border border-[color:var(--ink-900)]/20 bg-[color:var(--background-elevated)] px-3 text-sm text-foreground placeholder:text-foreground-tertiary transition-colors focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]";

  return (
    <div className="flex flex-col gap-3 border-b border-border-subtle py-4 md:flex-row md:items-start md:gap-4">
      <div className="md:w-56 md:shrink-0">
        <label className="sr-only" htmlFor={`option-name-${option.id ?? option.name}`}>
          Option name
        </label>
        <input
          id={`option-name-${option.id ?? option.name}`}
          type="text"
          value={option.name}
          onChange={(e) => onNameChange(e.target.value)}
          placeholder="Option name"
          className={fieldClass}
        />
      </div>
      <div className="flex flex-1 flex-col gap-1">
        <div className="flex min-h-10 flex-wrap items-center gap-1.5 rounded-md border border-[color:var(--ink-900)]/20 bg-[color:var(--background-elevated)] px-2 py-1.5 focus-within:border-[color:var(--moss-700)] focus-within:ring-1 focus-within:ring-[color:var(--moss-700)]">
          {option.values.map((v, i) => (
            <span
              key={v.id ?? `${i}-${v.value}`}
              className="inline-flex items-center gap-1 rounded-full bg-[color:var(--ink-900)]/[0.06] px-2.5 py-1 text-xs text-foreground"
            >
              {v.value}
              <button
                type="button"
                aria-label={`Remove value ${v.value}`}
                onClick={() => removeValue(i)}
                className="text-foreground-tertiary transition-colors hover:text-foreground focus:text-foreground focus:outline-none"
              >
                <X className="h-3 w-3" />
              </button>
            </span>
          ))}
          <input
            type="text"
            value={inflight}
            onChange={(e) => setInflight(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                commitValue();
              }
            }}
            onBlur={commitValue}
            placeholder={option.values.length === 0 ? "Add a value" : "Add another"}
            className="min-w-[8rem] flex-1 bg-transparent px-1.5 py-0.5 text-sm text-foreground placeholder:text-foreground-tertiary outline-none"
          />
        </div>
        <p className="text-[11px] text-foreground-tertiary">
          Press Enter to add each value
        </p>
      </div>
      <button
        type="button"
        aria-label="Remove option"
        onClick={onRemove}
        className="inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-md text-foreground-tertiary transition-colors hover:bg-[color:var(--ink-900)]/[0.04] hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
      >
        <Trash2 className="h-4 w-4" />
      </button>
    </div>
  );
}
