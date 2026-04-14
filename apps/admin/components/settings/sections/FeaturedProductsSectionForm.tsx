"use client";

import type { AdminCategory, HomepageSection } from "@/lib/api/marketplace-api";
import { FieldLabel, TextInput } from "../BrandingSettingsClient";

type FeaturedSection = Extract<HomepageSection, { type: "featured_products" }>;

interface Props {
  section: FeaturedSection;
  onChange: (next: FeaturedSection) => void;
  categories: Pick<AdminCategory, "id" | "slug" | "name">[];
  editable: boolean;
}

export function FeaturedProductsSectionForm({
  section,
  onChange,
  categories,
  editable,
}: Props) {
  const patch = (p: Partial<FeaturedSection>) => onChange({ ...section, ...p });
  const known = categories.some((c) => c.slug === section.collection_slug);

  return (
    <div className="space-y-4">
      <div className="space-y-1.5">
        <FieldLabel htmlFor="section_featured_heading">Heading (optional)</FieldLabel>
        <TextInput
          id="section_featured_heading"
          value={section.heading ?? ""}
          onChange={(v) => patch({ heading: v || null })}
          placeholder="Featured this month"
          disabled={!editable}
          maxLength={200}
        />
      </div>
      <div className="space-y-1.5">
        <FieldLabel htmlFor="section_featured_collection">Collection</FieldLabel>
        {categories.length === 0 ? (
          <p className="text-xs text-foreground-secondary">
            No collections yet. Create one under <strong>Catalog → Collections</strong>.
          </p>
        ) : (
          <select
            id="section_featured_collection"
            className="h-10 w-full rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] px-3 text-sm text-foreground disabled:opacity-50"
            value={section.collection_slug}
            onChange={(e) => patch({ collection_slug: e.target.value })}
            disabled={!editable}
          >
            <option value="">Pick a collection…</option>
            {categories.map((c) => (
              <option key={c.id} value={c.slug}>
                {c.name}
              </option>
            ))}
          </select>
        )}
        {section.collection_slug && !known ? (
          <p className="mt-1 text-xs text-danger">
            This collection (<code>{section.collection_slug}</code>) no longer exists.
            Customers won&apos;t see anything here until you pick another.
          </p>
        ) : null}
      </div>
      <div className="space-y-1.5">
        <FieldLabel htmlFor="section_featured_limit">Max products (1–24)</FieldLabel>
        <input
          id="section_featured_limit"
          type="number"
          min={1}
          max={24}
          value={section.limit ?? 8}
          onChange={(e) => patch({ limit: Number(e.target.value) || 8 })}
          disabled={!editable}
          className="h-10 w-24 rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] px-3 text-sm text-foreground disabled:opacity-50"
        />
      </div>
    </div>
  );
}
