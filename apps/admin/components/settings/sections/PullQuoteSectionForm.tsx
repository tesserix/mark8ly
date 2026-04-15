"use client";

import type { HomepageSection } from "@/lib/api/marketplace-api";
import { FieldLabel, TextInput } from "../BrandingSettingsClient";

type PullQuoteSection = Extract<HomepageSection, { type: "pull_quote" }>;

interface Props {
  section: PullQuoteSection;
  onChange: (next: PullQuoteSection) => void;
  editable: boolean;
}

export function PullQuoteSectionForm({ section, onChange, editable }: Props) {
  const patch = (p: Partial<PullQuoteSection>) => onChange({ ...section, ...p });
  return (
    <div className="space-y-4">
      <div className="space-y-1.5">
        <FieldLabel htmlFor="section_pullquote_text">Quote</FieldLabel>
        <textarea
          id="section_pullquote_text"
          rows={3}
          value={section.text}
          onChange={(e) => patch({ text: e.target.value })}
          disabled={!editable}
          maxLength={500}
          placeholder="A short, memorable line."
          className="w-full rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] px-3 py-2 text-sm text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)] disabled:opacity-50"
        />
        <p className="text-xs text-foreground-secondary">
          Rendered as an editorial pull quote — styling varies by layout.
        </p>
      </div>
      <div className="space-y-1.5">
        <FieldLabel htmlFor="section_pullquote_attribution">Attribution (optional)</FieldLabel>
        <TextInput
          id="section_pullquote_attribution"
          value={section.attribution ?? ""}
          onChange={(v) => patch({ attribution: v || null })}
          placeholder="Editor's note · Your Store"
          disabled={!editable}
          maxLength={200}
        />
      </div>
    </div>
  );
}
