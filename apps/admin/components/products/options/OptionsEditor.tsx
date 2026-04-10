"use client";

import * as React from "react";
import { OptionRow } from "./OptionRow";

export interface OptionDraft {
  id?: string;
  name: string;
  values: Array<{ id?: string; value: string }>;
}

export interface OptionsEditorProps {
  value: OptionDraft[];
  onChange: (next: OptionDraft[]) => void;
}

export function OptionsEditor({ value, onChange }: OptionsEditorProps): React.ReactElement {
  const updateAt = (index: number, next: OptionDraft): void => {
    onChange(value.map((opt, i) => (i === index ? next : opt)));
  };

  const removeAt = (index: number): void => {
    onChange(value.filter((_, i) => i !== index));
  };

  const addOption = (): void => {
    onChange([...value, { name: "", values: [] }]);
  };

  return (
    <div className="flex flex-col">
      {value.map((opt, i) => (
        <OptionRow
          key={opt.id ?? `row-${i}`}
          option={opt}
          onNameChange={(name) => updateAt(i, { ...opt, name })}
          onValuesChange={(values) => updateAt(i, { ...opt, values })}
          onRemove={() => removeAt(i)}
        />
      ))}
      <button
        type="button"
        onClick={addOption}
        className="mt-3 self-start text-xs uppercase tracking-widest text-[var(--ink-500)] hover:text-[var(--moss-700)] focus:text-[var(--moss-700)] focus:outline-none"
      >
        + Add option
      </button>
    </div>
  );
}
