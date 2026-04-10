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

  return (
    <div className="flex flex-col gap-3 border-b border-[var(--ink-100)] py-4 md:flex-row md:items-start md:gap-6">
      <div className="md:w-48">
        <label className="sr-only" htmlFor={`option-name-${option.id ?? option.name}`}>
          Option name
        </label>
        <input
          id={`option-name-${option.id ?? option.name}`}
          type="text"
          value={option.name}
          onChange={(e) => onNameChange(e.target.value)}
          placeholder="Option name"
          className="w-full bg-[var(--paper-200)] px-3 py-2 text-sm text-[var(--ink-900)] outline-none focus:ring-2 focus:ring-[var(--moss-700)]"
        />
      </div>
      <div className="flex flex-1 flex-wrap items-center gap-2">
        {option.values.map((v, i) => (
          <span
            key={v.id ?? `${i}-${v.value}`}
            className="inline-flex items-center gap-1 rounded-full border border-[var(--ink-100)] bg-[var(--background-elevated)] px-3 py-1 text-xs text-[var(--ink-900)]"
          >
            {v.value}
            <button
              type="button"
              aria-label={`Remove value ${v.value}`}
              onClick={() => removeValue(i)}
              className="text-[var(--ink-500)] hover:text-[var(--moss-700)] focus:text-[var(--moss-700)] focus:outline-none"
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
          placeholder="Add a value"
          className="min-w-[8rem] flex-1 bg-transparent px-2 py-1 text-xs text-[var(--ink-900)] outline-none focus:ring-2 focus:ring-[var(--moss-700)]"
        />
      </div>
      <button
        type="button"
        aria-label="Remove option"
        onClick={onRemove}
        className="self-start text-[var(--ink-500)] hover:text-[var(--moss-700)] focus:text-[var(--moss-700)] focus:outline-none"
      >
        <Trash2 className="h-4 w-4" />
      </button>
    </div>
  );
}
