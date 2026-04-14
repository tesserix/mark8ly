"use client";

import type { HomepageSection } from "@/lib/api/marketplace-api";
import { FieldLabel, TextInput } from "../BrandingSettingsClient";

type QuoteSection = Extract<HomepageSection, { type: "quote" }>;

interface Props {
  section: QuoteSection;
  onChange: (next: QuoteSection) => void;
  editable: boolean;
}

export function QuoteSectionForm({ section, onChange, editable }: Props) {
  const patch = (p: Partial<QuoteSection>) => onChange({ ...section, ...p });
  return (
    <div className="space-y-4">
      <div className="space-y-1.5">
        <FieldLabel htmlFor="section_quote_heading">Heading (optional)</FieldLabel>
        <TextInput
          id="section_quote_heading"
          value={section.heading ?? ""}
          onChange={(v) => patch({ heading: v || null })}
          placeholder="What they say"
          disabled={!editable}
          maxLength={200}
        />
      </div>
      <div className="space-y-1.5">
        <FieldLabel htmlFor="section_quote_text">Quote</FieldLabel>
        <textarea
          id="section_quote_text"
          rows={4}
          value={section.text}
          onChange={(e) => patch({ text: e.target.value })}
          disabled={!editable}
          maxLength={500}
          className="w-full rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] px-3 py-2 text-sm text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)] disabled:opacity-50"
        />
      </div>
      <div className="space-y-1.5">
        <FieldLabel htmlFor="section_quote_attribution">Attribution (optional)</FieldLabel>
        <TextInput
          id="section_quote_attribution"
          value={section.attribution ?? ""}
          onChange={(v) => patch({ attribution: v || null })}
          placeholder="Vogue, 2026"
          disabled={!editable}
          maxLength={200}
        />
      </div>
    </div>
  );
}
