"use client";

import type { HomepageSection } from "@/lib/api/marketplace-api";
import { FieldLabel, TextInput } from "../BrandingSettingsClient";

type ImageSection = Extract<HomepageSection, { type: "image" }>;

interface Props {
  section: ImageSection;
  onChange: (next: ImageSection) => void;
  editable: boolean;
}

export function ImageSectionForm({ section, onChange, editable }: Props) {
  const patch = (p: Partial<ImageSection>) => onChange({ ...section, ...p });
  return (
    <div className="space-y-4">
      <div className="space-y-1.5">
        <FieldLabel htmlFor="section_image_heading">Heading (optional)</FieldLabel>
        <TextInput
          id="section_image_heading"
          value={section.heading ?? ""}
          onChange={(v) => patch({ heading: v || null })}
          placeholder="A close look"
          disabled={!editable}
          maxLength={200}
        />
      </div>
      <div className="space-y-1.5">
        <FieldLabel htmlFor="section_image_url">Image URL</FieldLabel>
        <TextInput
          id="section_image_url"
          value={section.url}
          onChange={(v) => patch({ url: v })}
          placeholder="https://cdn.example.com/photo.jpg"
          disabled={!editable}
        />
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-1.5">
          <FieldLabel htmlFor="section_image_alt">Alt text</FieldLabel>
          <TextInput
            id="section_image_alt"
            value={section.alt ?? ""}
            onChange={(v) => patch({ alt: v || null })}
            placeholder="Describe what's in the image"
            disabled={!editable}
            maxLength={200}
          />
        </div>
        <div className="space-y-1.5">
          <FieldLabel htmlFor="section_image_caption">Caption</FieldLabel>
          <TextInput
            id="section_image_caption"
            value={section.caption ?? ""}
            onChange={(v) => patch({ caption: v || null })}
            placeholder="Shot by our team in Kyoto"
            disabled={!editable}
            maxLength={200}
          />
        </div>
      </div>
    </div>
  );
}
