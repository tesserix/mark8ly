"use client";

import type { HomepageSection } from "@/lib/api/marketplace-api";
import { FieldLabel, TextInput } from "../BrandingSettingsClient";
import { Markdown } from "@/lib/markdown";

type TextSection = Extract<HomepageSection, { type: "text" }>;

interface Props {
  section: TextSection;
  onChange: (next: TextSection) => void;
  editable: boolean;
}

export function TextSectionForm({ section, onChange, editable }: Props) {
  const patch = (p: Partial<TextSection>) => onChange({ ...section, ...p });

  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <div className="space-y-4">
        <div className="space-y-1.5">
          <FieldLabel htmlFor="section_text_heading">Heading (optional)</FieldLabel>
          <TextInput
            id="section_text_heading"
            value={section.heading ?? ""}
            onChange={(v) => patch({ heading: v || null })}
            placeholder="About our craft"
            disabled={!editable}
            maxLength={200}
          />
        </div>
        <div className="space-y-1.5">
          <FieldLabel htmlFor="section_text_markdown">Body (markdown)</FieldLabel>
          <textarea
            id="section_text_markdown"
            rows={14}
            value={section.markdown}
            onChange={(e) => patch({ markdown: e.target.value })}
            disabled={!editable}
            maxLength={20_000}
            className="w-full rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] px-3 py-2 font-mono text-sm text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)] disabled:opacity-50"
          />
        </div>
      </div>
      <aside>
        <p className="text-xs uppercase tracking-wide text-foreground-secondary">Preview</p>
        <div className="mt-2 rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] px-6 py-6">
          {section.heading ? (
            <h3 className="font-serif text-xl font-medium text-foreground">{section.heading}</h3>
          ) : null}
          <Markdown className={"prose max-w-none " + (section.heading ? "mt-4" : "") + " prose-headings:font-serif prose-a:text-moss-700"}>
            {section.markdown || "_Start writing to see the preview…_"}
          </Markdown>
        </div>
      </aside>
    </div>
  );
}
