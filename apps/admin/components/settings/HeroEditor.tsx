"use client";

import type { HomepageHero, AdminPage } from "@/lib/api/marketplace-api";
import {
  FieldLabel,
  SectionHeader,
  TextInput,
  ToggleSwitch,
} from "./BrandingSettingsClient";

interface HeroEditorProps {
  value: HomepageHero;
  onChange: (next: HomepageHero) => void;
  pages: Pick<AdminPage, "id" | "slug" | "title">[];
  editable: boolean;
}

export function HeroEditor({ value, onChange, pages, editable }: HeroEditorProps) {
  const patch = (p: Partial<HomepageHero>) => onChange({ ...value, ...p });
  const enabled = value.enabled;

  return (
    <div className="space-y-6">
      <SectionHeader
        title="Hero"
        description="The banner at the very top of your homepage. Leave fields blank to fall back to your store name."
      />

      <div className="flex items-center justify-between rounded-[var(--radius)] border border-border px-4 py-3">
        <div>
          <p className="text-sm font-medium text-foreground">Show hero</p>
          <p className="mt-0.5 text-xs text-foreground-secondary">
            Hide to suppress the hero entirely — storefront renders only the sections below.
          </p>
        </div>
        <ToggleSwitch
          checked={enabled}
          onChange={(v) => patch({ enabled: v })}
          disabled={!editable}
        />
      </div>

      {enabled ? (
        <div className="space-y-5">
          <div className="space-y-1.5">
            <FieldLabel htmlFor="hero_heading">Heading</FieldLabel>
            <TextInput
              id="hero_heading"
              value={value.heading ?? ""}
              onChange={(v) => patch({ heading: v || null })}
              placeholder="Welcome to our store"
              disabled={!editable}
              maxLength={200}
            />
          </div>

          <div className="space-y-1.5">
            <FieldLabel htmlFor="hero_subheading">Subheading</FieldLabel>
            <TextInput
              id="hero_subheading"
              value={value.subheading ?? ""}
              onChange={(v) => patch({ subheading: v || null })}
              placeholder="A short line about what you make"
              disabled={!editable}
              maxLength={400}
            />
          </div>

          <div className="space-y-1.5">
            <FieldLabel htmlFor="hero_image">Image URL</FieldLabel>
            <TextInput
              id="hero_image"
              value={value.image_url ?? ""}
              onChange={(v) => patch({ image_url: v || null })}
              placeholder="https://cdn.example.com/hero.jpg"
              disabled={!editable}
            />
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1.5">
              <FieldLabel htmlFor="hero_cta_label">CTA label</FieldLabel>
              <TextInput
                id="hero_cta_label"
                value={value.cta_label ?? ""}
                onChange={(v) => patch({ cta_label: v || null })}
                placeholder="Shop now"
                disabled={!editable}
                maxLength={60}
              />
            </div>
            <div className="space-y-1.5">
              <FieldLabel htmlFor="hero_cta_url">CTA destination</FieldLabel>
              <CtaUrlPicker
                value={value.cta_url ?? ""}
                onChange={(v) => patch({ cta_url: v || null })}
                pages={pages}
                disabled={!editable}
              />
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}

// CtaUrlPicker — a hybrid control: a <select> of pages plus a freeform
// URL field. Picking a page sets /pages/{slug}; typing in the freeform
// field overrides. Mirrors the UX of FooterSectionsEditor's item pickers.
function CtaUrlPicker({
  value,
  onChange,
  pages,
  disabled,
}: {
  value: string;
  onChange: (v: string) => void;
  pages: Pick<AdminPage, "id" | "slug" | "title">[];
  disabled?: boolean;
}) {
  const matched = pages.find((p) => `/pages/${p.slug}` === value);

  return (
    <div className="space-y-2">
      {pages.length > 0 ? (
        <select
          className="h-10 w-full rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] px-3 text-sm text-foreground disabled:opacity-50"
          value={matched ? matched.slug : ""}
          onChange={(e) => {
            const slug = e.target.value;
            if (slug) onChange(`/pages/${slug}`);
          }}
          disabled={disabled}
        >
          <option value="">Pick a page…</option>
          {pages.map((p) => (
            <option key={p.id} value={p.slug}>
              {p.title}
            </option>
          ))}
        </select>
      ) : (
        <p className="text-xs text-foreground-secondary">
          No pages yet. Create one under Settings → Pages, or paste a URL below.
        </p>
      )}
      <TextInput
        id="hero_cta_url_freeform"
        value={value}
        onChange={onChange}
        placeholder="https://example.com or /collections/featured"
        disabled={disabled}
      />
    </div>
  );
}
