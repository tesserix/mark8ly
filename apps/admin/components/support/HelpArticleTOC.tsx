"use client";

import { useEffect, useState } from "react";

import type { HelpHeading } from "@/lib/help";

interface HelpArticleTOCProps {
  headings: HelpHeading[];
}

export function HelpArticleTOC({ headings }: HelpArticleTOCProps) {
  const [activeId, setActiveId] = useState<string | null>(
    headings[0]?.slug ?? null,
  );

  useEffect(() => {
    if (headings.length === 0) return;

    const observer = new IntersectionObserver(
      (entries) => {
        const visible = entries
          .filter((e) => e.isIntersecting)
          .sort(
            (a, b) =>
              a.target.getBoundingClientRect().top -
              b.target.getBoundingClientRect().top,
          );
        if (visible[0]?.target.id) {
          setActiveId(visible[0].target.id);
        }
      },
      {
        // Trigger when heading sits in the upper portion of the viewport.
        rootMargin: "-96px 0px -55% 0px",
        threshold: 0,
      },
    );

    for (const h of headings) {
      const el = document.getElementById(h.slug);
      if (el) observer.observe(el);
    }
    return () => observer.disconnect();
  }, [headings]);

  if (headings.length === 0) return null;

  return (
    <nav aria-label="On this page" className="text-sm">
      <p className="eyebrow">On this page</p>
      <ul className="mt-3 space-y-1" role="list">
        {headings.map((h) => {
          const isActive = activeId === h.slug;
          return (
            <li key={h.slug}>
              <a
                href={`#${h.slug}`}
                className={[
                  "block border-l-2 py-1 transition-colors",
                  h.level === 3 ? "pl-6" : "pl-3",
                  isActive
                    ? "border-[color:var(--moss-700)] font-medium text-foreground"
                    : "border-border-subtle text-foreground-secondary hover:border-foreground-tertiary hover:text-foreground",
                ].join(" ")}
              >
                {h.text}
              </a>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
