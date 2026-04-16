"use client";

import type { HomepageSection } from "@/lib/api/marketplace-api";
import { FieldLabel, TextInput } from "../BrandingSettingsClient";
import { PageBodyEditor } from "../PageBodyEditor";

type TextSection = Extract<HomepageSection, { type: "text" }>;

interface Props {
  section: TextSection;
  onChange: (next: TextSection) => void;
  editable: boolean;
}

export function TextSectionForm({ section, onChange, editable }: Props) {
  const patch = (p: Partial<TextSection>) => onChange({ ...section, ...p });

  return (
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
        <FieldLabel htmlFor="section_text_markdown">Body</FieldLabel>
        <PageBodyEditor
          markdown={section.markdown}
          onChange={(md) => patch({ markdown: md })}
          editable={editable}
        />
      </div>
    </div>
  );
}
