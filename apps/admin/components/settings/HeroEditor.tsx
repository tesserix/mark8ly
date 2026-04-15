"use client";

import { useEffect, useRef } from "react";
import type { HomepageHero, AdminPage } from "@/lib/api/marketplace-api";
import {
  FieldLabel,
  SectionHeader,
  TextInput,
  ToggleSwitch,
} from "./BrandingSettingsClient";

// Mirrors the storefront heroStylesFor() matrix — intentionally duplicated
// to keep the admin bundle free of storefront imports.
const THEME_HERO_SUPPORT: Record<string, { eyebrow: boolean; secondaryCta: boolean; aside: boolean }> = {
  editorial:    { eyebrow: true,  secondaryCta: true,  aside: true  },
  "bold-promo": { eyebrow: true,  secondaryCta: true,  aside: false },
  "split-hero": { eyebrow: true,  secondaryCta: false, aside: true  },
  "story-led":  { eyebrow: true,  secondaryCta: false, aside: true  },
  minimal:      { eyebrow: true,  secondaryCta: false, aside: false },
  "classic-shop":  { eyebrow: true,  secondaryCta: false, aside: false },
  "catalog-first": { eyebrow: true,  secondaryCta: false, aside: false },
  compact:      { eyebrow: false, secondaryCta: false, aside: false },
};

interface HeroEditorProps {
  value: HomepageHero;
  onChange: (next: HomepageHero) => void;
  pages: Pick<AdminPage, "id" | "slug" | "title">[];
  editable: boolean;
  layoutVariant?: string;
  // Validity signal: called whenever the aside-alt a11y constraint changes.
  // valid=false when aside_image_url is set but aside_image_alt is empty.
  onValidityChange?: (valid: boolean) => void;
}

export function HeroEditor({ value, onChange, pages, editable, layoutVariant, onValidityChange }: HeroEditorProps) {
  const patch = (p: Partial<HomepageHero>) => onChange({ ...value, ...p });
  const enabled = value.enabled;

  const asideImageSet = Boolean(value.aside_image_url);
  const asideAltMissing = asideImageSet && !value.aside_image_alt;

  const secondaryLabelSet = Boolean(value.cta_secondary_label);
  const secondaryUrlSet = Boolean(value.cta_secondary_url);
  const secondaryIncomplete = secondaryLabelSet !== secondaryUrlSet;

  // Notify parent whenever the form validity state changes. Gates Save
  // on both the a11y rule (aside alt required when aside URL set) and
  // the paired-CTA rule (label + URL set together or both empty).
  //
  // The callback is captured through a ref so the effect fires only on
  // real validity transitions — stability of the parent's onValidityChange
  // prop is not required (e.g. inline closures won't cause a re-run).
  const onValidityChangeRef = useRef(onValidityChange);
  useEffect(() => {
    onValidityChangeRef.current = onValidityChange;
  });
  useEffect(() => {
    onValidityChangeRef.current?.(!asideAltMissing && !secondaryIncomplete);
  }, [asideAltMissing, secondaryIncomplete]);

  const themeSupport = layoutVariant ? THEME_HERO_SUPPORT[layoutVariant] : undefined;

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
            <FieldLabel htmlFor="hero_eyebrow">Eyebrow</FieldLabel>
            <TextInput
              id="hero_eyebrow"
              value={value.eyebrow ?? ""}
              onChange={(v) => patch({ eyebrow: v || null })}
              placeholder="Issue 01 · Now open"
              disabled={!editable}
              maxLength={80}
            />
            <p className="text-xs text-foreground-secondary">
              A short kicker above the heading (e.g. &apos;Issue 01 · Now open&apos;). Not shown on all themes.
            </p>
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
                inputId="hero_cta_url_freeform"
              />
            </div>
          </div>

          {/* Secondary CTA section */}
          <div className="space-y-4 border-t border-border-subtle pt-5">
            <p className="text-xs font-semibold uppercase tracking-wide text-foreground-secondary">
              Secondary CTA
            </p>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-1.5">
                <FieldLabel htmlFor="hero_cta_secondary_label">Label</FieldLabel>
                <TextInput
                  id="hero_cta_secondary_label"
                  value={value.cta_secondary_label ?? ""}
                  onChange={(v) => patch({ cta_secondary_label: v || null })}
                  placeholder="Learn more"
                  disabled={!editable}
                  maxLength={60}
                />
              </div>
              <div className="space-y-1.5">
                <FieldLabel htmlFor="hero_cta_secondary_url">Destination</FieldLabel>
                <CtaUrlPicker
                  value={value.cta_secondary_url ?? ""}
                  onChange={(v) => patch({ cta_secondary_url: v || null })}
                  pages={pages}
                  disabled={!editable}
                  inputId="hero_cta_secondary_url_freeform"
                />
              </div>
            </div>
            {secondaryIncomplete ? (
              <p className="text-xs text-[color:var(--signal)]">
                Secondary CTA requires both a label and a URL
              </p>
            ) : null}
          </div>

          {/* Aside image section */}
          <div className="space-y-4 border-t border-border-subtle pt-5">
            <p className="text-xs font-semibold uppercase tracking-wide text-foreground-secondary">
              Aside image
            </p>
            <div className="space-y-1.5">
              <FieldLabel htmlFor="hero_aside_image_url">Image URL</FieldLabel>
              <TextInput
                id="hero_aside_image_url"
                value={value.aside_image_url ?? ""}
                onChange={(v) => patch({ aside_image_url: v || null })}
                placeholder="https://cdn.example.com/aside.jpg"
                disabled={!editable}
                maxLength={500}
              />
            </div>
            <div className="space-y-1.5">
              <FieldLabel htmlFor="hero_aside_image_alt">
                Alt text{asideAltMissing ? <span className="ml-1 text-[color:var(--danger)]">*</span> : null}
              </FieldLabel>
              <TextInput
                id="hero_aside_image_alt"
                value={value.aside_image_alt ?? ""}
                onChange={(v) => patch({ aside_image_alt: v || null })}
                placeholder="A model wearing the signature jacket"
                disabled={!editable}
                maxLength={200}
              />
              {asideAltMissing ? (
                <p className="text-xs text-[color:var(--danger)]">
                  Alt text is required when an aside image is set — customers using assistive tech need this
                </p>
              ) : null}
            </div>
          </div>

          {/* Theme hint */}
          {layoutVariant ? (
            <p className="text-xs text-foreground-secondary">
              Your theme ({layoutVariant}) renders:{" "}
              eyebrow {themeSupport?.eyebrow ? "✓" : "✗"} ·{" "}
              secondary CTA {themeSupport?.secondaryCta ? "✓" : "✗"} ·{" "}
              aside image {themeSupport?.aside ? "✓" : "✗"}
            </p>
          ) : null}
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
  inputId,
}: {
  value: string;
  onChange: (v: string) => void;
  pages: Pick<AdminPage, "id" | "slug" | "title">[];
  disabled?: boolean;
  inputId: string;
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
        id={inputId}
        value={value}
        onChange={onChange}
        placeholder="https://example.com or /collections/featured"
        disabled={disabled}
        maxLength={500}
      />
    </div>
  );
}
