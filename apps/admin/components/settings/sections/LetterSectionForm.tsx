"use client";

import type { HomepageSection } from "@/lib/api/marketplace-api";
import { FieldLabel, TextInput } from "../BrandingSettingsClient";
import { PageBodyEditor } from "../PageBodyEditor";

type LetterSection = Extract<HomepageSection, { type: "letter" }>;

interface Props {
  section: LetterSection;
  onChange: (next: LetterSection) => void;
  editable: boolean;
}

const MAX_EYEBROW = 120;
const MAX_TITLE = 200;
const MAX_BODY = 10_000;
const MAX_CTA_LABEL = 60;

export function LetterSectionForm({ section, onChange, editable }: Props) {
  const patch = (p: Partial<LetterSection>) => onChange({ ...section, ...p });
  return (
    <div className="space-y-4">
      <div className="space-y-1.5">
        <FieldLabel htmlFor="section_letter_eyebrow">Eyebrow (optional)</FieldLabel>
        <TextInput
          id="section_letter_eyebrow"
          value={section.eyebrow ?? ""}
          onChange={(v) => patch({ eyebrow: v || null })}
          placeholder="Letter from the studio"
          disabled={!editable}
          maxLength={MAX_EYEBROW}
        />
      </div>
      <div className="space-y-1.5">
        <FieldLabel htmlFor="section_letter_title">Title</FieldLabel>
        <TextInput
          id="section_letter_title"
          value={section.title}
          onChange={(v) => patch({ title: v })}
          placeholder="Built for the long shelf life."
          disabled={!editable}
          maxLength={MAX_TITLE}
        />
      </div>
      <div className="space-y-1.5">
        <FieldLabel htmlFor="section_letter_body">Body</FieldLabel>
        <PageBodyEditor
          markdown={section.body}
          onChange={(md) => patch({ body: md })}
          editable={editable}
        />
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-1.5">
          <FieldLabel htmlFor="section_letter_cta_label">CTA label (optional)</FieldLabel>
          <TextInput
            id="section_letter_cta_label"
            value={section.cta_label ?? ""}
            onChange={(v) => patch({ cta_label: v || null })}
            placeholder="About the studio"
            disabled={!editable}
            maxLength={MAX_CTA_LABEL}
          />
        </div>
        <div className="space-y-1.5">
          <FieldLabel htmlFor="section_letter_cta_url">CTA URL (optional)</FieldLabel>
          <TextInput
            id="section_letter_cta_url"
            value={section.cta_url ?? ""}
            onChange={(v) => patch({ cta_url: v || null })}
            placeholder="/pages/about"
            disabled={!editable}
            maxLength={2048}
          />
          <p className="text-xs text-foreground-secondary">
            Relative (<code>/pages/about</code>) or https URLs only.
          </p>
        </div>
      </div>
    </div>
  );
}
