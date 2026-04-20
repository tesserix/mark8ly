"use client";

import type { AdminCategory, HomepageSection } from "@/lib/api/marketplace-api";
import { FieldLabel, TextInput } from "../BrandingSettingsClient";
import { ProductSlugPicker } from "../ProductSlugPicker";

type FeaturedSection = Extract<HomepageSection, { type: "featured_products" }>;

type PicksMode = "collection" | "handpicked";

const MAX_HAND_PICKED = 6;

interface Props {
  section: FeaturedSection;
  onChange: (next: FeaturedSection) => void;
  categories: Pick<AdminCategory, "id" | "slug" | "name">[];
  editable: boolean;
  storeId: string;
}

function inferMode(section: FeaturedSection): PicksMode {
  return section.product_slugs && section.product_slugs.length > 0
    ? "handpicked"
    : "collection";
}

export function FeaturedProductsSectionForm({
  section,
  onChange,
  categories,
  editable,
  storeId,
}: Props) {
  const patch = (p: Partial<FeaturedSection>) => onChange({ ...section, ...p });
  const mode = inferMode(section);
  const known = categories.some((c) => c.slug === section.collection_slug);

  const switchToCollection = () => {
    onChange({
      type: "featured_products",
      heading: section.heading ?? null,
      limit: section.limit,
      collection_slug: section.collection_slug ?? "",
    });
  };
  const switchToHandpicked = () => {
    onChange({
      type: "featured_products",
      heading: section.heading ?? null,
      limit: section.limit,
      product_slugs: section.product_slugs ?? [],
    });
  };

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

      <fieldset className="space-y-2">
        <FieldLabel>Source</FieldLabel>
        <div className="inline-flex rounded-md border border-border bg-[color:var(--background-elevated)] p-0.5 text-sm">
          <button
            type="button"
            onClick={switchToCollection}
            disabled={!editable}
            className={
              mode === "collection"
                ? "rounded-md bg-[color:var(--ink-900)]/5 px-3 py-1.5 font-medium text-foreground"
                : "rounded-md px-3 py-1.5 text-foreground-secondary hover:text-foreground"
            }
          >
            By collection
          </button>
          <button
            type="button"
            onClick={switchToHandpicked}
            disabled={!editable}
            className={
              mode === "handpicked"
                ? "rounded-md bg-[color:var(--ink-900)]/5 px-3 py-1.5 font-medium text-foreground"
                : "rounded-md px-3 py-1.5 text-foreground-secondary hover:text-foreground"
            }
          >
            Hand-picked
          </button>
        </div>
      </fieldset>

      {mode === "collection" ? (
        <div className="space-y-1.5">
          <FieldLabel htmlFor="section_featured_collection">Collection</FieldLabel>
          {categories.length === 0 ? (
            <p className="text-xs text-foreground-secondary">
              No collections yet. Create one under <strong>Catalog → Collections</strong>.
            </p>
          ) : (
            <select
              id="section_featured_collection"
              className="h-10 w-full rounded-md border border-border bg-[color:var(--background-elevated)] px-3 text-sm text-foreground disabled:opacity-50"
              value={section.collection_slug ?? ""}
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
      ) : (
        <ProductSlugPicker
          storeId={storeId}
          value={section.product_slugs ?? []}
          onChange={(slugs) => patch({ product_slugs: slugs })}
          max={MAX_HAND_PICKED}
          editable={editable}
          label="Products (in order)"
        />
      )}

      {mode === "collection" ? (
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
            className="h-10 w-24 rounded-md border border-border bg-[color:var(--background-elevated)] px-3 text-sm text-foreground disabled:opacity-50"
          />
        </div>
      ) : null}
    </div>
  );
}
