"use client";

import { useId, useState } from "react";

export interface FaqItem {
  question: string;
  answer: string;
}

interface FaqAccordionProps {
  items: FaqItem[];
  /** Optional className for the outer <ul>. */
  className?: string;
}

/**
 * FaqAccordion — accessible, animated, headless-styled FAQ list.
 *
 * Why this exists instead of <details>/<summary>:
 *  - We need controlled "only one open at a time" semantics
 *  - We need a smooth open/close animation that does NOT animate
 *    layout properties (no max-height tricks)
 *
 * The animation uses `grid-template-rows: 0fr → 1fr` which is
 * the modern, layout-safe way to animate from 0 to natural height.
 *
 * Accessibility:
 *  - Each panel has a stable id, linked to its trigger via
 *    `aria-controls`
 *  - Trigger exposes `aria-expanded`
 *  - Collapsed panels are marked `hidden` so they leave the
 *    accessibility tree entirely (not just visually clipped)
 *
 * Styling: visual style comes from the consuming app via
 * Tailwind classes on the wrapper. The component only owns
 * structure, state, and a11y.
 */
export function FaqAccordion({ items, className }: FaqAccordionProps) {
  const [openIndex, setOpenIndex] = useState<number | null>(null);
  const baseId = useId();

  return (
    <ul className={className}>
      {items.map((item, i) => {
        const isOpen = openIndex === i;
        const triggerId = `${baseId}-trigger-${i}`;
        const panelId = `${baseId}-panel-${i}`;

        return (
          <li
            key={item.question}
            data-state={isOpen ? "open" : "closed"}
            className="faq-accordion__item border-b border-border-subtle"
          >
            <h3 className="faq-accordion__heading">
              <button
                id={triggerId}
                type="button"
                aria-expanded={isOpen}
                aria-controls={panelId}
                onClick={() => setOpenIndex(isOpen ? null : i)}
                className="faq-accordion__trigger group flex w-full items-baseline justify-between gap-6 py-6 text-left transition-colors duration-200"
              >
                <span className="text-lg text-foreground sm:text-xl">
                  {item.question}
                </span>
                <span
                  aria-hidden="true"
                  className="mt-1 flex h-6 w-6 flex-shrink-0 items-center justify-center text-foreground-tertiary transition-transform duration-300 ease-out group-aria-expanded:rotate-45"
                >
                  <svg
                    width="16"
                    height="16"
                    viewBox="0 0 16 16"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="1.5"
                    strokeLinecap="round"
                  >
                    <path d="M8 2v12M2 8h12" />
                  </svg>
                </span>
              </button>
            </h3>
            <div
              id={panelId}
              role="region"
              aria-labelledby={triggerId}
              hidden={!isOpen}
              className="faq-accordion__panel grid transition-[grid-template-rows] duration-300 ease-out"
              style={{
                gridTemplateRows: isOpen ? "1fr" : "0fr",
              }}
            >
              <div className="overflow-hidden">
                <p className="pb-6 pr-12 text-foreground-secondary leading-relaxed">
                  {item.answer}
                </p>
              </div>
            </div>
          </li>
        );
      })}
    </ul>
  );
}
