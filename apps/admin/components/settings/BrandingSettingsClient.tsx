"use client";

import { useState, useTransition, useCallback, type ReactNode } from "react";
import { Link2, Code, Image, Check, Home, Palette, FileText } from "lucide-react";

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
  // Rendered inside the Theme tab. Passed in as a slot so the page
  // owns the <StorefrontThemeForm> instance (needs the full Store,
  // which BrandingSettingsClient doesn't have).
  themeContent?: ReactNode;
  // Rendered inside the Pages tab. PagesList needs the full AdminPage
  // list (not the trimmed summaries used for the footer editor), so
  // the parent server component owns the data fetch + render.
  pagesContent?: ReactNode;
}

type Tab = "identity" | "theme" | "homepage" | "pages" | "footer" | "advanced";

const TABS: { key: Tab; label: string; icon: typeof Image }[] = [
  { key: "identity", label: "Identity", icon: Image },
  { key: "theme",    label: "Theme",    icon: Palette },
  { key: "homepage", label: "Homepage", icon: Home },
  { key: "pages",    label: "Pages",    icon: FileText },
  { key: "footer",   label: "Footer",   icon: Link2 },
  { key: "advanced", label: "Advanced", icon: Code },
];

// Colors, Typography, and Layout are now configured exclusively via the
// Storefront Theme form on this same page. The legacy per-column fields,
// FONTS list, LAYOUTS list, and COLOR_PRESETS list that used to live
// here have been removed in favour of the preset + surface + layout
// picker with live preview. Color mirror writes happen server-side in
// updateStorefrontTheme so existing storefront components that still
// read branding.color_* keep working.

// ─── Component ──────────────────────────────────────────────────────

export function BrandingSettingsClient({
  branding: initial,
  editable,
  pages = [],
  categories = [],
  store,
  themeContent,
  pagesContent,
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
    <div className="space-y-8">
      {/* Horizontal tab bar — reclaims the full content width for the
          main editing surface. Underline style, Shopify/Linear-like.
          No overflow-x-auto — the tab set is bounded (4 tabs), so a
          scrollbar is never needed and fires false on wide screens. */}
      <nav
        className="flex flex-wrap gap-1 border-b border-border-subtle"
        aria-label="Branding sections"
      >
        {TABS.map(({ key, label, icon: Icon }) => {
          const active = tab === key;
          return (
            <button
              key={key}
              type="button"
              onClick={() => setTab(key)}
              aria-pressed={active}
              className={`relative flex items-center gap-2 whitespace-nowrap px-4 py-3 text-sm transition-colors ${
                active
                  ? "font-medium text-foreground"
                  : "text-foreground-secondary hover:text-foreground"
              }`}
            >
              <Icon className="h-4 w-4 shrink-0" aria-hidden="true" />
              {label}
              {active ? (
                <span
                  aria-hidden
                  className="absolute inset-x-2 -bottom-px h-[2px] rounded-t-sm bg-[color:var(--moss-700)]"
                />
              ) : null}
            </button>
          );
        })}
      </nav>

      {/* Main content */}
      <div className="min-w-0 space-y-8">
        {tab === "identity" && <IdentityTab form={form} patch={patch} editable={editable} />}
        {tab === "theme" && themeContent}
        {tab === "homepage" && <HomepageTab form={form} patch={patch} editable={editable} pages={pages} categories={categories} store={store} onHeroValidityChange={setHeroValid} />}
        {tab === "pages" && pagesContent}
        {tab === "footer" && <FooterTab form={form} patch={patch} editable={editable} pages={pages} />}
        {tab === "advanced" && <AdvancedTab form={form} patch={patch} editable={editable} />}

        {/* Save bar — hidden on the Theme tab, which ships its own
            save/reset/preview controls inside the StorefrontThemeForm. */}
        {editable && tab !== "theme" && (
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

export function ColorField({
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

