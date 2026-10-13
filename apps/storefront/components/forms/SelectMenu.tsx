"use client";

// Themed replacement for a native <select>. The OS menu ignores the
// merchant's theme entirely (macOS draws it grey and translucent over a
// cream storefront), so this renders the same popover as the address
// suggestions in AddressFieldset.
//
// Keyboard follows the WAI-ARIA select-only combobox pattern: arrows move,
// Enter/Space choose, Escape closes, Home/End jump, and typing a letter
// jumps to the next option starting with it.

import { useEffect, useId, useRef, useState } from "react";

export interface SelectMenuOption {
  value: string;
  label: string;
  /** Leading glyph, e.g. a flag emoji. Decorative. */
  icon?: string;
}

interface SelectMenuProps {
  id: string;
  value: string;
  options: SelectMenuOption[];
  onChange: (value: string) => void;
  placeholder: string;
  disabled?: boolean;
  className: string;
}

export function SelectMenu({
  id,
  value,
  options,
  onChange,
  placeholder,
  disabled = false,
  className,
}: SelectMenuProps) {
  const listId = useId();
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(-1);
  const rootRef = useRef<HTMLDivElement>(null);
  const listRef = useRef<HTMLUListElement>(null);

  const selected = options.find((o) => o.value === value);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [open]);

  useEffect(() => {
    if (!open || active < 0) return;
    listRef.current
      ?.querySelector<HTMLElement>(`[data-index="${active}"]`)
      ?.scrollIntoView({ block: "nearest" });
  }, [open, active]);

  function openMenu() {
    setActive(Math.max(0, options.findIndex((o) => o.value === value)));
    setOpen(true);
  }

  function choose(i: number) {
    const o = options[i];
    if (o) onChange(o.value);
    setOpen(false);
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLButtonElement>) {
    if (!open) {
      if (["ArrowDown", "ArrowUp", "Enter", " "].includes(e.key)) {
        e.preventDefault();
        openMenu();
      }
      return;
    }
    const last = options.length - 1;
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        setActive((i) => Math.min(last, i + 1));
        return;
      case "ArrowUp":
        e.preventDefault();
        setActive((i) => Math.max(0, i - 1));
        return;
      case "Home":
        e.preventDefault();
        setActive(0);
        return;
      case "End":
        e.preventDefault();
        setActive(last);
        return;
      case "Enter":
      case " ":
        e.preventDefault();
        choose(active);
        return;
      case "Escape":
      case "Tab":
        setOpen(false);
        return;
    }
    if (e.key.length === 1 && /\S/.test(e.key)) {
      const ch = e.key.toLowerCase();
      for (let step = 1; step <= options.length; step++) {
        const i = (active + step) % options.length;
        if (options[i]?.label.toLowerCase().startsWith(ch)) {
          setActive(i);
          return;
        }
      }
    }
  }

  return (
    <div ref={rootRef} className="relative">
      <button
        id={id}
        type="button"
        role="combobox"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={listId}
        aria-activedescendant={open && active >= 0 ? `${listId}-${active}` : undefined}
        disabled={disabled}
        onClick={() => (open ? setOpen(false) : openMenu())}
        onKeyDown={onKeyDown}
        className={`${className} flex items-center gap-2 text-left`}
      >
        {selected ? (
          <>
            {selected.icon && <span aria-hidden>{selected.icon}</span>}
            <span className="truncate">{selected.label}</span>
          </>
        ) : (
          <span className="truncate opacity-40">{placeholder}</span>
        )}
        <svg
          aria-hidden
          viewBox="0 0 16 16"
          className={`ml-auto h-3.5 w-3.5 shrink-0 opacity-60 transition-transform ${open ? "rotate-180" : ""}`}
        >
          <path d="M4 6l4 4 4-4" fill="none" stroke="currentColor" strokeWidth="1.5" />
        </svg>
      </button>
      {open && (
        <ul
          ref={listRef}
          id={listId}
          role="listbox"
          aria-labelledby={id}
          className="absolute z-20 mt-1 max-h-64 w-full overflow-auto rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/10 bg-[color:var(--storefront-surface)] py-1 shadow-lg"
        >
          {options.map((o, i) => (
            <li
              key={o.value}
              id={`${listId}-${i}`}
              data-index={i}
              role="option"
              aria-selected={o.value === value}
              onMouseDown={(e) => {
                e.preventDefault();
                choose(i);
              }}
              onMouseEnter={() => setActive(i)}
              className={`flex cursor-pointer items-center gap-2 px-3 py-2 text-sm text-[color:var(--storefront-text,var(--ink-900))] ${
                i === active ? "bg-[color:var(--storefront-accent,var(--moss-700))]/[0.08]" : ""
              } ${o.value === value ? "font-medium" : ""}`}
            >
              {o.icon && <span aria-hidden>{o.icon}</span>}
              <span>{o.label}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
