"use client";

import type { HomepageSection } from "@/lib/api/marketplace-api";
import { FieldLabel, TextInput } from "../BrandingSettingsClient";
import { ImageUploadInput } from "../ImageUploadInput";

type ImageSection = Extract<HomepageSection, { type: "image" }>;

interface Props {
  section: ImageSection;
  onChange: (next: ImageSection) => void;
  editable: boolean;
  storeId: string;
}

export function ImageSectionForm({ section, onChange, editable, storeId }: Props) {
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
        <FieldLabel>Image</FieldLabel>
        <ImageUploadInput
          storeId={storeId}
          kind="section"
          value={section.url || null}
          onChange={(url) => patch({ url: url ?? "" })}
          disabled={!editable}
          hint="PNG, JPG, WebP — up to 5 MB."
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
