"use client";

import { useState, useTransition, useCallback } from "react";
import {
  Palette,
  Type,
  LayoutGrid,
  Link2,
  Code,
  Image,
  Check,
  Home,
} from "lucide-react";

import type { StoreBranding, UpdateBrandingInput, AdminPage, AdminCategory } from "@/lib/api/marketplace-api";
import { HomepageTab } from "./HomepageTab";
import type { StoreInfo } from "./HomepageTab.helpers";
import { FooterSectionsEditor } from "./FooterSectionsEditor";
import { ImageUploadInput } from "./ImageUploadInput";
import { updateBrandingAction } from "@/app/(admin)/settings/themes/actions";
import { useToast } from "@/components/feedback/Toaster";

// ─── Types ──────────────────────────────────────────────────────────

type AdminPageSummary = Pick<AdminPage, "id" | "slug" | "title">;

interface BrandingSettingsClientProps {
  branding: StoreBranding;
  editable: boolean;
  pages?: AdminPageSummary[];
  categories?: Pick<AdminCategory, "id" | "slug" | "name">[];
  store: StoreInfo;
}

type Tab =
  | "identity"
  | "colors"
  | "typography"
  | "layout"
  | "homepage"
  | "footer"
  | "advanced";

const TABS: { key: Tab; label: string; icon: typeof Palette }[] = [
  { key: "identity", label: "Identity", icon: Image },
  { key: "colors", label: "Colors", icon: Palette },
  { key: "typography", label: "Typography", icon: Type },
  { key: "layout", label: "Layout", icon: LayoutGrid },
  { key: "homepage", label: "Homepage", icon: Home },
  { key: "footer", label: "Footer", icon: Link2 },
  { key: "advanced", label: "Advanced", icon: Code },
];

const FONTS: { key: string; label: string; family: string }[] = [
  // Serif — editorial + body
  { key: "source-serif-4",   label: "Source Serif 4",   family: "'Source Serif 4', Georgia, serif" },
  { key: "newsreader",       label: "Newsreader",       family: "'Newsreader', 'Iowan Old Style', Georgia, serif" },
  { key: "playfair-display", label: "Playfair Display", family: "'Playfair Display', Georgia, serif" },
  { key: "cormorant",        label: "Cormorant",        family: "'Cormorant Garamond', Georgia, serif" },
  { key: "lora",             label: "Lora",             family: "'Lora', Georgia, serif" },
  { key: "libre-baskerville", label: "Libre Baskerville", family: "'Libre Baskerville', Baskerville, serif" },
  { key: "crimson",          label: "Crimson Pro",      family: "'Crimson Pro', 'Crimson Text', Georgia, serif" },
  // Sans — workhorse
  { key: "source-sans-3",    label: "Source Sans 3",    family: "'Source Sans 3', system-ui, sans-serif" },
  { key: "inter",            label: "Inter",            family: "'Inter', system-ui, sans-serif" },
  { key: "manrope",          label: "Manrope",          family: "'Manrope', 'Avenir Next', sans-serif" },
  { key: "dm-sans",          label: "DM Sans",          family: "'DM Sans', system-ui, sans-serif" },
  { key: "work-sans",        label: "Work Sans",        family: "'Work Sans', system-ui, sans-serif" },
  { key: "poppins",          label: "Poppins",          family: "'Poppins', system-ui, sans-serif" },
  { key: "ibm-plex-sans",    label: "IBM Plex Sans",    family: "'IBM Plex Sans', system-ui, sans-serif" },
  // Technical / grotesque
  { key: "space-grotesk",    label: "Space Grotesk",    family: "'Space Grotesk', 'Inter', sans-serif" },
  { key: "archivo",          label: "Archivo",          family: "'Archivo', 'Inter', sans-serif" },
];

const LAYOUTS: { key: string; label: string; desc: string }[] = [
  // Editorial / magazine
  { key: "editorial",          label: "Editorial",          desc: "Magazine-style with hero features and premium pacing" },
  { key: "story-led",          label: "Story-led",          desc: "Narrative flow with softer hierarchy" },
  { key: "lookbook",           label: "Lookbook",           desc: "Big-image tiles and editorial spreads" },
  // Shop / catalog
  { key: "classic-shop",       label: "Classic Shop",       desc: "Traditional grid with sidebar categories" },
  { key: "catalog-first",      label: "Catalog First",      desc: "Product-led landing with quick highlights" },
  { key: "grid-dense",         label: "Grid Dense",         desc: "Tight product grid — maximum SKUs above the fold" },
  { key: "collection-showcase", label: "Collection Showcase", desc: "Categories as featured tiles above products" },
  // Hero
  { key: "hero-focus",         label: "Hero Focus",         desc: "Large hero banner above featured products" },
  { key: "split-hero",         label: "Split Hero",         desc: "Left-right composition with stronger action" },
  { key: "product-spotlight",  label: "Product Spotlight",  desc: "Single hero product above a lean catalog" },
  { key: "bold-promo",         label: "Bold Promo",         desc: "Campaign-forward with stronger contrast" },
  // Long + quiet
  { key: "landing-story",      label: "Landing Story",      desc: "Long-scroll brand narrative with modular sections" },
  { key: "minimal",            label: "Minimal",            desc: "Clean, product-forward with open space" },
  { key: "compact",            label: "Compact",            desc: "Dense storefront for practical browsing" },
];

// Quick-apply presets for the Colors tab. Kept in sync with the
// modern palettes in @repo/ui/storefront-theme so the Colors tab and
// the Storefront Theme picker speak the same design language. Button
// bg/text mirror text/background for high-contrast CTAs.
const COLOR_PRESETS: { name: string; colors: Pick<StoreBranding, "color_background" | "color_text" | "color_accent" | "color_button_bg" | "color_button_text"> }[] = [
  // House
  { name: "Paper",         colors: { color_background: "#F7F6F2", color_text: "#0E0E0C", color_accent: "#2D4A2B", color_button_bg: "#0E0E0C", color_button_text: "#F7F6F2" } },
  // Warm
  { name: "Saffron Dusk",  colors: { color_background: "#FBF3E2", color_text: "#2C1A06", color_accent: "#C47C0A", color_button_bg: "#3D2208", color_button_text: "#FBF3E2" } },
  { name: "Marigold",      colors: { color_background: "#FFF0D0", color_text: "#271600", color_accent: "#B86800", color_button_bg: "#3B1E00", color_button_text: "#FFF0D0" } },
  { name: "Terracotta",    colors: { color_background: "#FAF0E8", color_text: "#2A1208", color_accent: "#C05A28", color_button_bg: "#3C1E0E", color_button_text: "#FAF0E8" } },
  { name: "Blush Studio",  colors: { color_background: "#FDF0F0", color_text: "#2A1010", color_accent: "#C03858", color_button_bg: "#3E1212", color_button_text: "#FDF0F0" } },
  { name: "Copper Roast",  colors: { color_background: "#F5EDE0", color_text: "#1E0E00", color_accent: "#9A4A00", color_button_bg: "#2E1600", color_button_text: "#F5EDE0" } },
  // Cool
  { name: "Arctic",        colors: { color_background: "#EDF4F7", color_text: "#0C1E26", color_accent: "#0D7FA5", color_button_bg: "#0F2D3A", color_button_text: "#EDF4F7" } },
  { name: "Pacific",       colors: { color_background: "#EBF0F5", color_text: "#0D1B2A", color_accent: "#1A5FA8", color_button_bg: "#0D2240", color_button_text: "#EBF0F5" } },
  { name: "Slate Cloud",   colors: { color_background: "#F0F1F5", color_text: "#141820", color_accent: "#3B5EE8", color_button_bg: "#1E2436", color_button_text: "#F0F1F5" } },
  // Bold / pop
  { name: "Citrus Burst",  colors: { color_background: "#FFF8D6", color_text: "#1E1600", color_accent: "#C88000", color_button_bg: "#2E2000", color_button_text: "#FFF8D6" } },
  { name: "Coral Pop",     colors: { color_background: "#FFF2EE", color_text: "#250D08", color_accent: "#D94832", color_button_bg: "#3A1008", color_button_text: "#FFF2EE" } },
  // Natural
  { name: "Greenhouse",    colors: { color_background: "#EDF5EE", color_text: "#0C1E10", color_accent: "#2A7A3C", color_button_bg: "#103018", color_button_text: "#EDF5EE" } },
  { name: "Matcha",        colors: { color_background: "#F2F5EC", color_text: "#141A0E", color_accent: "#5A7A28", color_button_bg: "#1E2E14", color_button_text: "#F2F5EC" } },
  // Pastel
  { name: "Lavender Mist", colors: { color_background: "#F3F0FA", color_text: "#1A1028", color_accent: "#6C40C8", color_button_bg: "#2A1848", color_button_text: "#F3F0FA" } },
  { name: "Sky Linen",     colors: { color_background: "#EBF4FC", color_text: "#0E1A24", color_accent: "#1A78C8", color_button_bg: "#122030", color_button_text: "#EBF4FC" } },
  // Monochrome
  { name: "Noir Pop",      colors: { color_background: "#EDEDEB", color_text: "#080808", color_accent: "#D41A1A", color_button_bg: "#0A0A0A", color_button_text: "#EDEDEB" } },
  // Jewel
  { name: "Plum Noir",     colors: { color_background: "#F0E8F5", color_text: "#1A0820", color_accent: "#8B2090", color_button_bg: "#2E0E40", color_button_text: "#F0E8F5" } },
  // Heritage
  { name: "Indigo Block",  colors: { color_background: "#EEF0F8", color_text: "#0C1030", color_accent: "#B07800", color_button_bg: "#12185A", color_button_text: "#EEF0F8" } },
  // Dark mode — admin-only selection; storefront renders the dark bg.
  { name: "Obsidian",      colors: { color_background: "#0A0A0A", color_text: "#EDEDED", color_accent: "#F5B800", color_button_bg: "#FAFAFA", color_button_text: "#0A0A0A" } },
  { name: "Velvet",        colors: { color_background: "#180B14", color_text: "#F0E5D8", color_accent: "#D9A652", color_button_bg: "#FBF5EA", color_button_text: "#180B14" } },
  { name: "Ember",         colors: { color_background: "#14100C", color_text: "#ECE4D4", color_accent: "#E6793E", color_button_bg: "#F5EFE3", color_button_text: "#14100C" } },
  { name: "Nebula",        colors: { color_background: "#0D0A24", color_text: "#E8E6F8", color_accent: "#D438C6", color_button_bg: "#F8F6FC", color_button_text: "#0D0A24" } },
  { name: "Aurora Dark",   colors: { color_background: "#081814", color_text: "#DDEFE5", color_accent: "#36E0A6", color_button_bg: "#F0FCF5", color_button_text: "#081814" } },
];

// ─── Component ──────────────────────────────────────────────────────

export function BrandingSettingsClient({
  branding: initial,
  editable,
  pages = [],
  categories = [],
  store,
}: BrandingSettingsClientProps) {
  const { toast } = useToast();
  const [form, setForm] = useState<StoreBranding>(initial);
  const [tab, setTab] = useState<Tab>("identity");
  const [isPending, startTransition] = useTransition();
  const [status, setStatus] = useState<{ type: "idle" | "saved" | "error"; message?: string }>({ type: "idle" });
  // heroValid tracks whether HeroEditor's aside-alt a11y constraint is satisfied.
  // Defaults true so the Save button isn't blocked before the homepage tab is visited.
  const [heroValid, setHeroValid] = useState(true);

  const dirty = JSON.stringify(form) !== JSON.stringify(initial);

  const patch = useCallback(
    (updates: Partial<StoreBranding>) => {
      setForm((prev) => ({ ...prev, ...updates }));
      setStatus({ type: "idle" });
    },
    [],
  );

  function handleReset() {
    setForm(initial);
    setStatus({ type: "idle" });
  }

  function handleSave() {
    setStatus({ type: "idle" });
    startTransition(async () => {
      const input: UpdateBrandingInput = {};
      const keys = Object.keys(form) as (keyof StoreBranding)[];
      for (const key of keys) {
        if (key === "id" || key === "store_id" || key === "created_at" || key === "updated_at") continue;
        if (form[key] !== initial[key]) {
          (input as Record<string, unknown>)[key] = form[key];
        }
      }
      const result = await updateBrandingAction(input);
      if (result.ok) {
        setStatus({ type: "saved" });
        toast.success("Branding saved", "Changes are live on your storefront.");
        setTimeout(() => setStatus({ type: "idle" }), 3000);
      } else {
        setStatus({ type: "error", message: result.message });
        toast.error("Couldn't save branding", result.message);
      }
    });
  }

  return (
    <div className="grid gap-10 lg:grid-cols-[220px_1fr]">
      {/* Left rail nav */}
      <nav className="flex flex-row gap-1 overflow-x-auto lg:flex-col lg:gap-0.5" aria-label="Branding sections">
        {TABS.map(({ key, label, icon: Icon }) => (
          <button
            key={key}
            type="button"
            onClick={() => setTab(key)}
            className={`flex items-center gap-2.5 rounded-[var(--radius)] px-3 py-2 text-left text-sm transition-colors ${
              tab === key
                ? "bg-[color:var(--ink-900)]/[0.04] font-medium text-foreground"
                : "text-foreground-secondary hover:text-foreground hover:bg-[color:var(--ink-900)]/[0.02]"
            }`}
          >
            <Icon className="h-4 w-4 shrink-0" aria-hidden="true" />
            <span className="whitespace-nowrap">{label}</span>
          </button>
        ))}
      </nav>

      {/* Main content */}
      <div className="min-w-0 space-y-8">
        {tab === "identity" && <IdentityTab form={form} patch={patch} editable={editable} />}
        {tab === "colors" && <ColorsTab form={form} patch={patch} editable={editable} />}
        {tab === "typography" && <TypographyTab form={form} patch={patch} editable={editable} />}
        {tab === "layout" && <LayoutTab form={form} patch={patch} editable={editable} />}
        {tab === "homepage" && <HomepageTab form={form} patch={patch} editable={editable} pages={pages} categories={categories} store={store} onHeroValidityChange={setHeroValid} />}
        {tab === "footer" && <FooterTab form={form} patch={patch} editable={editable} pages={pages} />}
        {tab === "advanced" && <AdvancedTab form={form} patch={patch} editable={editable} />}

        {/* Save bar */}
        {editable && (
          <div className="flex items-center gap-3 border-t border-border-subtle pt-6">
            <button
              type="button"
              onClick={handleSave}
              disabled={!dirty || isPending || !heroValid}
              className="inline-flex h-10 items-center gap-2 rounded-[var(--radius)] bg-[color:var(--ink-900)] px-5 text-sm font-medium text-white transition-colors hover:bg-[color:var(--ink-900)]/90 disabled:opacity-40"
            >
              {isPending ? "Saving..." : status.type === "saved" ? (
                <>
                  <Check className="h-3.5 w-3.5" aria-hidden="true" />
                  Saved
                </>
              ) : "Save changes"}
            </button>
            {dirty && (
              <button
                type="button"
                onClick={handleReset}
                disabled={isPending}
                className="h-10 rounded-[var(--radius)] px-4 text-sm text-foreground-secondary transition-colors hover:text-foreground disabled:opacity-40"
              >
                Reset
              </button>
            )}
            {status.type === "error" && (
              <p role="alert" className="text-sm text-[color:var(--signal)]">
                {status.message}
              </p>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

// ─── Shared pieces ──────────────────────────────────────────────────

interface TabProps {
  form: StoreBranding;
  patch: (u: Partial<StoreBranding>) => void;
  editable: boolean;
  pages?: AdminPageSummary[];
  categories?: Pick<AdminCategory, "id" | "slug" | "name">[];
}

export function SectionHeader({ title, description }: { title: string; description: string }) {
  return (
    <div className="space-y-1">
      <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium tracking-tight text-foreground">
        {title}
      </h2>
      <p className="text-sm leading-6 text-foreground-secondary">{description}</p>
    </div>
  );
}

export function FieldLabel({ htmlFor, children }: { htmlFor?: string; children: React.ReactNode }) {
  return (
    <label htmlFor={htmlFor} className="block text-sm font-medium text-foreground">
      {children}
    </label>
  );
}

export function TextInput({
  id,
  value,
  onChange,
  placeholder,
  disabled,
  maxLength,
}: {
  id: string;
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  disabled?: boolean;
  maxLength?: number;
}) {
  return (
    <input
      id={id}
      type="text"
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      disabled={disabled}
      maxLength={maxLength}
      className="h-10 w-full rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] px-3 text-sm text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)] disabled:opacity-50"
    />
  );
}

function ColorField({
  id,
  label,
  value,
  onChange,
  disabled,
}: {
  id: string;
  label: string;
  value: string;
  onChange: (v: string) => void;
  disabled?: boolean;
}) {
  return (
    <div className="space-y-1.5">
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <div className="flex items-center gap-2">
        <input
          type="color"
          id={id}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          disabled={disabled}
          className="h-10 w-10 shrink-0 cursor-pointer rounded-[var(--radius)] border border-border p-0.5 disabled:cursor-default disabled:opacity-50"
        />
        <input
          type="text"
          value={value}
          onChange={(e) => {
            const v = e.target.value;
            if (/^#[0-9A-Fa-f]{0,6}$/.test(v)) onChange(v);
          }}
          disabled={disabled}
          className="h-10 w-24 rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] px-3 font-mono text-xs text-foreground focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)] disabled:opacity-50"
        />
      </div>
    </div>
  );
}

// ─── Identity Tab ───────────────────────────────────────────────────

function IdentityTab({ form, patch, editable }: TabProps) {
  return (
    <div className="space-y-8">
      <SectionHeader
        title="Identity"
        description="Your store's logo, favicon, and tagline. These appear across your storefront and browser tabs."
      />

      <div className="space-y-5">
        <div className="space-y-1.5">
          <FieldLabel>Logo</FieldLabel>
          <ImageUploadInput
            storeId={form.store_id}
            kind="logo"
            value={form.logo_url}
            onChange={(url) => patch({ logo_url: url })}
            disabled={!editable}
            hint="SVG, PNG, JPG, or WebP — up to 1 MB. At least 400px wide. Appears in storefront header."
          />
        </div>

        <div className="space-y-1.5">
          <FieldLabel>Favicon</FieldLabel>
          <ImageUploadInput
            storeId={form.store_id}
            kind="favicon"
            value={form.favicon_url}
            onChange={(url) => patch({ favicon_url: url })}
            disabled={!editable}
            hint="32×32 PNG, ICO, or SVG — up to 1 MB. Shown in browser tabs and bookmarks."
          />
        </div>

        <div className="space-y-1.5">
          <FieldLabel htmlFor="tagline">Tagline</FieldLabel>
          <TextInput
            id="tagline"
            value={form.tagline ?? ""}
            onChange={(v) => patch({ tagline: v || null })}
            placeholder="Crafted with care, delivered with love"
            disabled={!editable}
            maxLength={200}
          />
          <p className="text-xs text-foreground-tertiary">
            Shown below your store name. Max 200 characters.
          </p>
        </div>
      </div>
    </div>
  );
}

// ─── Colors Tab ─────────────────────────────────────────────────────

function ColorsTab({ form, patch, editable }: TabProps) {
  return (
    <div className="space-y-8">
      <SectionHeader
        title="Colors"
        description="Define your storefront palette. All combinations are checked for WCAG AA contrast compliance."
      />

      {/* Presets */}
      <div className="space-y-3">
        <p className="text-sm font-medium text-foreground">Quick presets</p>
        <div className="flex flex-wrap gap-2">
          {COLOR_PRESETS.map((preset) => {
            const active =
              form.color_background === preset.colors.color_background &&
              form.color_text === preset.colors.color_text &&
              form.color_accent === preset.colors.color_accent;
            return (
              <button
                key={preset.name}
                type="button"
                onClick={() => editable && patch(preset.colors)}
                disabled={!editable}
                className={`group flex items-center gap-2 rounded-full border px-3 py-1.5 text-xs transition-all disabled:opacity-50 ${
                  active
                    ? "border-[color:var(--moss-700)] bg-[color:var(--moss-700)]/[0.06] font-medium text-[color:var(--moss-700)]"
                    : "border-border text-foreground-secondary hover:border-foreground-tertiary"
                }`}
              >
                <span className="flex gap-0.5">
                  {[preset.colors.color_background, preset.colors.color_text, preset.colors.color_accent].map((c) => (
                    <span
                      key={c}
                      className="h-3 w-3 rounded-full border border-[color:var(--ink-900)]/10"
                      style={{ backgroundColor: c }}
                    />
                  ))}
                </span>
                {preset.name}
              </button>
            );
          })}
        </div>
      </div>

      {/* Individual color pickers */}
      <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
        <ColorField id="color_background" label="Background" value={form.color_background} onChange={(v) => patch({ color_background: v })} disabled={!editable} />
        <ColorField id="color_text" label="Text" value={form.color_text} onChange={(v) => patch({ color_text: v })} disabled={!editable} />
        <ColorField id="color_accent" label="Accent" value={form.color_accent} onChange={(v) => patch({ color_accent: v })} disabled={!editable} />
        <ColorField id="color_button_bg" label="Button background" value={form.color_button_bg} onChange={(v) => patch({ color_button_bg: v })} disabled={!editable} />
        <ColorField id="color_button_text" label="Button text" value={form.color_button_text} onChange={(v) => patch({ color_button_text: v })} disabled={!editable} />
      </div>

      {/* Live preview swatch */}
      <div className="space-y-2">
        <p className="text-sm font-medium text-foreground">Preview</p>
        <div
          className="overflow-hidden rounded-[var(--radius-md)] border border-border"
          style={{ backgroundColor: form.color_background }}
        >
          <div className="px-6 py-5 space-y-3">
            <p className="text-lg font-medium" style={{ color: form.color_text, fontFamily: "'Source Serif 4', Georgia, serif" }}>
              Your Storefront
            </p>
            <p className="text-sm" style={{ color: form.color_text }}>
              This is how body text appears on your background.{" "}
              <span style={{ color: form.color_accent }}>Accent links look like this.</span>
            </p>
            <button
              type="button"
              disabled
              className="h-9 rounded-[var(--radius)] px-4 text-sm font-medium"
              style={{ backgroundColor: form.color_button_bg, color: form.color_button_text }}
            >
              Add to cart
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

// ─── Typography Tab ─────────────────────────────────────────────────

function TypographyTab({ form, patch, editable }: TabProps) {
  return (
    <div className="space-y-8">
      <SectionHeader
        title="Typography"
        description="Choose heading and body typefaces. Each option is rendered in its actual font."
      />

      <div className="grid gap-6 sm:grid-cols-2">
        <div className="space-y-3">
          <FieldLabel>Heading font</FieldLabel>
          <div className="space-y-1.5">
            {FONTS.filter((f) => f.key.includes("serif") || f.key.includes("playfair") || f.key.includes("lora")).map((f) => (
              <FontOption
                key={f.key}
                font={f}
                selected={form.heading_font === f.key}
                onSelect={() => editable && patch({ heading_font: f.key })}
                disabled={!editable}
              />
            ))}
          </div>
        </div>
        <div className="space-y-3">
          <FieldLabel>Body font</FieldLabel>
          <div className="space-y-1.5">
            {FONTS.filter((f) => f.key.includes("sans") || f.key === "inter").map((f) => (
              <FontOption
                key={f.key}
                font={f}
                selected={form.body_font === f.key}
                onSelect={() => editable && patch({ body_font: f.key })}
                disabled={!editable}
              />
            ))}
          </div>
        </div>
      </div>

      {/* Typography preview */}
      <div className="space-y-2">
        <p className="text-sm font-medium text-foreground">Preview</p>
        <div className="rounded-[var(--radius-md)] border border-border bg-[color:var(--background-elevated)] px-6 py-5 space-y-2">
          <p
            className="text-2xl font-medium tracking-tight"
            style={{ fontFamily: FONTS.find((f) => f.key === form.heading_font)?.family }}
          >
            The quick brown fox jumps
          </p>
          <p
            className="text-sm leading-6 text-foreground-secondary"
            style={{ fontFamily: FONTS.find((f) => f.key === form.body_font)?.family }}
          >
            Over the lazy dog. Typography is the craft of endowing human language with a durable visual form, and thus with an independent existence.
          </p>
        </div>
      </div>
    </div>
  );
}

function FontOption({
  font,
  selected,
  onSelect,
  disabled,
}: {
  font: { key: string; label: string; family: string };
  selected: boolean;
  onSelect: () => void;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      disabled={disabled}
      className={`flex w-full items-center justify-between rounded-[var(--radius)] border px-3 py-2.5 text-left transition-colors disabled:opacity-50 ${
        selected
          ? "border-[color:var(--moss-700)] bg-[color:var(--moss-700)]/[0.04]"
          : "border-border hover:border-foreground-tertiary"
      }`}
    >
      <span className="text-sm" style={{ fontFamily: font.family }}>
        {font.label}
      </span>
      {selected && <Check className="h-3.5 w-3.5 text-[color:var(--moss-700)]" aria-hidden="true" />}
    </button>
  );
}

// ─── Layout Tab ─────────────────────────────────────────────────────

function LayoutTab({ form, patch, editable }: TabProps) {
  return (
    <div className="space-y-8">
      <SectionHeader
        title="Layout & homepage"
        description="Choose your storefront layout and configure the homepage hero and announcement bar."
      />

      {/* Layout variants */}
      <div className="space-y-3">
        <FieldLabel>Storefront layout</FieldLabel>
        <div className="grid gap-3 sm:grid-cols-2">
          {LAYOUTS.map((l) => (
            <button
              key={l.key}
              type="button"
              onClick={() => editable && patch({ layout_variant: l.key })}
              disabled={!editable}
              className={`rounded-[var(--radius)] border p-4 text-left transition-colors disabled:opacity-50 ${
                form.layout_variant === l.key
                  ? "border-[color:var(--moss-700)] bg-[color:var(--moss-700)]/[0.04]"
                  : "border-border hover:border-foreground-tertiary"
              }`}
            >
              <p className="text-sm font-medium text-foreground">{l.label}</p>
              <p className="mt-0.5 text-xs text-foreground-secondary">{l.desc}</p>
            </button>
          ))}
        </div>
      </div>

      {/* Announcement bar */}
      <div className="space-y-4 border-t border-border-subtle pt-6">
        <div className="flex items-center justify-between">
          <div className="space-y-0.5">
            <p className="text-sm font-medium text-foreground">Announcement bar</p>
            <p className="text-xs text-foreground-secondary">Display a promotional message at the top of your storefront.</p>
          </div>
          <ToggleSwitch
            checked={form.announcement_active}
            onChange={(v) => editable && patch({ announcement_active: v })}
            disabled={!editable}
          />
        </div>
        {form.announcement_active && (
          <div className="space-y-4 pl-0">
            <div className="space-y-1.5">
              <FieldLabel htmlFor="announcement_text">Message</FieldLabel>
              <TextInput
                id="announcement_text"
                value={form.announcement_text ?? ""}
                onChange={(v) => patch({ announcement_text: v || null })}
                placeholder="Free shipping on orders over $50"
                disabled={!editable}
                maxLength={300}
              />
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-1.5">
                <FieldLabel htmlFor="announcement_link">Link (optional)</FieldLabel>
                <TextInput
                  id="announcement_link"
                  value={form.announcement_link ?? ""}
                  onChange={(v) => patch({ announcement_link: v || null })}
                  placeholder="/collections/sale"
                  disabled={!editable}
                />
              </div>
              <ColorField
                id="announcement_bg"
                label="Bar color"
                value={form.announcement_bg ?? form.color_accent}
                onChange={(v) => patch({ announcement_bg: v })}
                disabled={!editable}
              />
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

// ─── Footer Tab ─────────────────────────────────────────────────────

function FooterTab({ form, patch, editable, pages = [] }: TabProps) {
  return (
    <div className="space-y-8">
      <SectionHeader
        title="Footer"
        description="Configure the tagline, copyright notice, and social media links displayed in your storefront footer."
      />

      <div className="grid gap-5 sm:grid-cols-2">
        <div className="space-y-1.5 sm:col-span-2">
          <FieldLabel htmlFor="footer_tagline">Footer tagline</FieldLabel>
          <TextInput
            id="footer_tagline"
            value={form.footer_tagline ?? ""}
            onChange={(v) => patch({ footer_tagline: v || null })}
            placeholder="Quality goods, honestly priced"
            disabled={!editable}
            maxLength={300}
          />
        </div>
        <div className="space-y-1.5 sm:col-span-2">
          <FieldLabel htmlFor="footer_copyright">Copyright text</FieldLabel>
          <TextInput
            id="footer_copyright"
            value={form.footer_copyright ?? ""}
            onChange={(v) => patch({ footer_copyright: v || null })}
            placeholder="© 2026 Your Store. All rights reserved."
            disabled={!editable}
            maxLength={200}
          />
        </div>
      </div>

      {/* Footer menu */}
      <div className="space-y-4 border-t border-border-subtle pt-6">
        <SectionHeader
          title="Footer menu"
          description="Group links into columns for the storefront footer. Each item can point to a page you've authored or an external URL."
        />
        <FooterSectionsEditor
          sections={form.footer_sections ?? []}
          onChange={(next) => patch({ footer_sections: next })}
          pages={pages}
          editable={editable}
        />
      </div>

      {/* Social links */}
      <div className="space-y-4 border-t border-border-subtle pt-6">
        <p className="text-sm font-medium text-foreground">Social links</p>
        <div className="grid gap-4 sm:grid-cols-2">
          {([
            { key: "social_instagram" as const, label: "Instagram", placeholder: "https://instagram.com/yourstore" },
            { key: "social_twitter" as const, label: "X / Twitter", placeholder: "https://x.com/yourstore" },
            { key: "social_facebook" as const, label: "Facebook", placeholder: "https://facebook.com/yourstore" },
            { key: "social_tiktok" as const, label: "TikTok", placeholder: "https://tiktok.com/@yourstore" },
            { key: "social_youtube" as const, label: "YouTube", placeholder: "https://youtube.com/@yourstore" },
          ] as const).map(({ key, label, placeholder }) => (
            <div key={key} className="space-y-1.5">
              <FieldLabel htmlFor={key}>{label}</FieldLabel>
              <TextInput
                id={key}
                value={form[key] ?? ""}
                onChange={(v) => patch({ [key]: v || null })}
                placeholder={placeholder}
                disabled={!editable}
              />
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

// ─── Advanced Tab ───────────────────────────────────────────────────

function AdvancedTab({ form, patch, editable }: TabProps) {
  return (
    <div className="space-y-8">
      <SectionHeader
        title="Advanced"
        description="Custom CSS injection and branding controls. Custom CSS is available on Enterprise plans."
      />

      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <div className="space-y-0.5">
            <p className="text-sm font-medium text-foreground">Show &quot;Powered by mark8ly&quot;</p>
            <p className="text-xs text-foreground-secondary">
              Display the mark8ly badge in the storefront footer. Pro plans and above can remove this.
            </p>
          </div>
          <ToggleSwitch
            checked={form.show_powered_by}
            onChange={(v) => editable && patch({ show_powered_by: v })}
            disabled={!editable}
          />
        </div>
      </div>

      <div className="space-y-3 border-t border-border-subtle pt-6">
        <FieldLabel htmlFor="custom_css">Custom CSS</FieldLabel>
        <p className="text-xs text-foreground-secondary">
          Inject CSS directly into your storefront. External URLs and JavaScript expressions are stripped for security.
        </p>
        <textarea
          id="custom_css"
          value={form.custom_css ?? ""}
          onChange={(e) => patch({ custom_css: e.target.value || null })}
          disabled={!editable}
          rows={12}
          spellCheck={false}
          className="w-full rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] px-3 py-2.5 font-mono text-xs leading-5 text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)] disabled:opacity-50"
          placeholder={`.storefront-header {\n  /* Your custom styles */\n}`}
        />
      </div>
    </div>
  );
}

// ─── Toggle Switch ──────────────────────────────────────────────────

export function ToggleSwitch({
  checked,
  onChange,
  disabled,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      onClick={() => !disabled && onChange(!checked)}
      disabled={disabled}
      className={`relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors disabled:opacity-50 ${
        checked ? "bg-[color:var(--moss-700)]" : "bg-[color:var(--ink-900)]/15"
      }`}
    >
      <span
        className={`inline-block h-4 w-4 transform rounded-full bg-white shadow-sm transition-transform ${
          checked ? "translate-x-6" : "translate-x-1"
        }`}
      />
    </button>
  );
}

