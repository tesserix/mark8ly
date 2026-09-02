"use client";

import { Checkbox } from "@tesserix/web";

import { EVENT_TYPE_GROUPS } from "./eventTypes";

interface EventTypeCheckboxGroupProps {
  value: string[];
  onChange: (next: string[]) => void;
  disabled?: boolean;
  /** Wires the error message to the fieldset for screen readers. */
  describedBy?: string;
}

/**
 * The 18 event types, grouped by aggregate (order / return / product /
 * category / cart) with the aggregate name as a group label — a flat list
 * of 18 checkboxes reads as noise, five short lists read as a menu.
 */
export function EventTypeCheckboxGroup({
  value,
  onChange,
  disabled,
  describedBy,
}: EventTypeCheckboxGroupProps) {
  const selected = new Set(value);

  function toggle(eventType: string, checked: boolean) {
    if (disabled) return;
    const next = new Set(selected);
    if (checked) {
      next.add(eventType);
    } else {
      next.delete(eventType);
    }
    onChange(Array.from(next));
  }

  return (
    <fieldset aria-describedby={describedBy} className="space-y-6">
      <legend className="text-sm font-medium text-foreground">Event types</legend>
      <div className="grid gap-6 sm:grid-cols-2">
        {EVENT_TYPE_GROUPS.map((group) => (
          <div key={group.aggregate} className="space-y-2.5">
            <p className="text-xs font-medium uppercase tracking-wide text-foreground-tertiary">
              {group.aggregate}
            </p>
            <ul className="space-y-2">
              {group.types.map((t) => {
                const id = `event-type-${t.value}`;
                return (
                  <li key={t.value} className="flex items-center gap-2.5">
                    <Checkbox
                      id={id}
                      checked={selected.has(t.value)}
                      onCheckedChange={(checked) => toggle(t.value, checked === true)}
                      disabled={disabled}
                    />
                    <label
                      htmlFor={id}
                      className="text-sm text-foreground-secondary"
                    >
                      {t.label}
                    </label>
                  </li>
                );
              })}
            </ul>
          </div>
        ))}
      </div>
    </fieldset>
  );
}
