"use client";

import { useMemo, useState, useTransition } from "react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@tesserix/web";
import { Field } from "@repo/ui/field";
import { StorefrontLayoutPreview, StorefrontLayoutThumbnail } from "@repo/ui/storefront-preview";

import { updateStorefrontTheme } from "@/app/(admin)/settings/themes/actions";
import { useToast } from "@/components/feedback/Toaster";
import type { Store } from "@/lib/api/platform-api";
import {
  defaultStorefrontTheme,
  normalizeStorefrontTheme,
  storefrontBackgroundOptions,
  storefrontFontOptions,
  storefrontLayoutOptions,
  storefrontPaletteOptions,
  withBackground,
  withColorOverride,
  withMode,
  withPalette,
  type StorefrontBackground,
  type StorefrontDensity,
  type StorefrontFont,
  type StorefrontMode,
  type StorefrontMotion,
  type StorefrontPalette,
  type StorefrontRadius,
  type StorefrontTheme,
} from "@repo/ui/storefront-theme";

interface StorefrontThemeFormProps {
  store: Store;
  editable?: boolean;
}

/**
 * StorefrontThemeForm — merchant-facing theme editor built on three
 * orthogonal controls: mode (light/dark), palette (brand identity),
 * background (neutral or tinted surface). Preset bundles are gone.
 *
 * Layout: colour and typography controls at the top, full-width
 * structural preview at the bottom. Advanced colour overrides are
 * collapsed by default.
 */
export function StorefrontThemeForm({
  store,
  editable = true,
}: StorefrontThemeFormProps) {
  const initialTheme = useMemo(
    () => normalizeStorefrontTheme(store.storefront_theme),
    [store.storefront_theme],
  );
  const [theme, setTheme] = useState<StorefrontTheme>(initialTheme);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [previewOpen, setPreviewOpen] = useState(false);
  const [pending, startTransition] = useTransition();
  const { toast } = useToast();

  const dirty = JSON.stringify(theme) !== JSON.stringify(initialTheme);

  const updateTheme = (next: StorefrontTheme) => {
    setTheme(next);
    setSuccess(false);
  };

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!dirty) return;
    setError(null);
    setSuccess(false);

    startTransition(async () => {
      const result = await updateStorefrontTheme(theme);
      if (!result.ok) {
        setError(result.message);
        toast.error("Couldn't save storefront theme", result.message);
        return;
      }
      setSuccess(true);
      toast.success("Storefront theme saved", "Changes are live on your storefront.");
      setTimeout(() => setSuccess(false), 3000);
    });
  }

  const disabled = !editable || pending;
  const backgroundOptions = storefrontBackgroundOptions[theme.mode];

  return (
    <form onSubmit={handleSubmit} className="space-y-14">
      {/* ── COLOUR ────────────────────────────────────────────── */}
      <section className="space-y-8 border-t border-border-subtle pt-10">
        <div className="space-y-2">
          <p className="eyebrow">Colour</p>
          <h2 className="font-serif text-2xl font-medium tracking-tight text-foreground">
            Mode, palette, background
          </h2>
          <p className="max-w-2xl text-sm leading-7 text-foreground-secondary">
            Three independent controls. Mode flips light/dark. Palette
            sets your brand accent. Background picks the surface behind
            product photography.
          </p>
        </div>

        {/* Mode */}
        <Field id="mode" label="Mode">
          <ModeToggle
            value={theme.mode}
            disabled={disabled}
            onChange={(mode) => updateTheme(withMode(theme, mode))}
          />
        </Field>

        {/* Palette */}
        <Field id="palette" label="Palette">
          <div className="flex flex-wrap gap-2 pt-1">
            {storefrontPaletteOptions.map((option) => (
              <PaletteChip
                key={option.value}
                option={option}
                active={theme.palette === option.value}
                mode={theme.mode}
                disabled={disabled}
                onClick={() => updateTheme(withPalette(theme, option.value))}
              />
            ))}
          </div>
        </Field>

        {/* Background */}
        <Field id="background" label="Background">
          <div className="flex flex-wrap gap-2 pt-1">
            {backgroundOptions.map((option) => {
              const active = theme.background === option.value;
              return (
                <button
                  key={option.value}
                  type="button"
                  disabled={disabled}
                  aria-pressed={active}
                  onClick={() => updateTheme(withBackground(theme, option.value))}
                  className={`min-h-[44px] rounded-md border px-4 text-left transition-colors ${
                    active
                      ? "border-moss-700 bg-moss-50"
                      : "border-border bg-background-elevated hover:border-border-strong"
                  } disabled:cursor-not-allowed disabled:opacity-60`}
                >
                  <p className="text-sm font-semibold text-foreground">
                    {option.label}
                  </p>
                  <p className="mt-0.5 text-xs text-foreground-secondary">
                    {option.description}
                  </p>
                </button>
              );
            })}
          </div>
          {theme.background === "custom" ? (
            <div className="pt-3">
              <ColorField
                label="Custom background hex"
                value={theme.colors.background}
                disabled={disabled}
                onChange={(v) => updateTheme(withColorOverride(theme, "background", v))}
              />
            </div>
          ) : null}
        </Field>

        {/* Advanced — collapsed by default */}
        <div className="border-t border-border-subtle pt-6">
          <button
            type="button"
            onClick={() => setAdvancedOpen((o) => !o)}
            className="text-sm font-medium text-foreground-secondary hover:text-foreground"
          >
            {advancedOpen ? "Hide" : "Show"} advanced colour overrides
          </button>
          {advancedOpen ? (
            <div className="mt-5 grid gap-5 sm:grid-cols-3">
              <ColorField
                label="Primary"
                value={theme.colors.primary}
                disabled={disabled}
                onChange={(v) => updateTheme(withColorOverride(theme, "primary", v))}
              />
              <ColorField
                label="Accent"
                value={theme.colors.accent}
                disabled={disabled}
                onChange={(v) => updateTheme(withColorOverride(theme, "accent", v))}
              />
              <ColorField
                label="Background"
                value={theme.colors.background}
                disabled={disabled}
                onChange={(v) => updateTheme(withColorOverride(theme, "background", v))}
              />
            </div>
          ) : null}
        </div>
      </section>

      {/* ── TYPOGRAPHY + LAYOUT ──────────────────────────────── */}
      <section className="space-y-8 border-t border-border-subtle pt-10">
        <div className="space-y-2">
          <p className="eyebrow">Typography &amp; layout</p>
          <h2 className="font-serif text-2xl font-medium tracking-tight text-foreground">
            Structure and type
          </h2>
        </div>

        <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
          <SelectField
            id="heading-font"
            label="Heading font"
            value={theme.typography.headingFont}
            options={storefrontFontOptions}
            disabled={disabled}
            onChange={(v) =>
              updateTheme({
                ...theme,
                typography: { ...theme.typography, headingFont: v as StorefrontFont },
              })
            }
          />
          <SelectField
            id="body-font"
            label="Body font"
            value={theme.typography.bodyFont}
            options={storefrontFontOptions}
            disabled={disabled}
            onChange={(v) =>
              updateTheme({
                ...theme,
                typography: { ...theme.typography, bodyFont: v as StorefrontFont },
              })
            }
          />
          <SelectField
            id="density"
            label="Density"
            value={theme.density}
            options={[
              { value: "compact", label: "Compact" },
              { value: "balanced", label: "Balanced" },
              { value: "airy", label: "Airy" },
            ]}
            disabled={disabled}
            onChange={(v) => updateTheme({ ...theme, density: v as StorefrontDensity })}
          />
          <SelectField
            id="radius"
            label="Corner style"
            value={theme.radius}
            options={[
              { value: "sharp", label: "Sharp" },
              { value: "soft", label: "Soft" },
              { value: "rounded", label: "Rounded" },
            ]}
            disabled={disabled}
            onChange={(v) => updateTheme({ ...theme, radius: v as StorefrontRadius })}
          />
          <SelectField
            id="motion"
            label="Motion"
            value={theme.motion}
            options={[
              { value: "none", label: "None" },
              { value: "subtle", label: "Subtle" },
              { value: "expressive", label: "Expressive" },
            ]}
            disabled={disabled}
            onChange={(v) => updateTheme({ ...theme, motion: v as StorefrontMotion })}
          />
        </div>

        <div className="space-y-3 pt-2">
          <p className="text-sm font-medium text-foreground">Layout</p>
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
            {storefrontLayoutOptions.map((option) => {
              const active = theme.layout === option.value;
              const cardTheme: StorefrontTheme = { ...theme, layout: option.value };
              return (
                <button
                  key={option.value}
                  type="button"
                  data-testid={`layout-${option.value}`}
                  aria-pressed={active}
                  disabled={disabled}
                  onClick={() => updateTheme({ ...theme, layout: option.value })}
                  className={`min-h-[44px] rounded-md border p-3 text-left transition-colors ${
                    active
                      ? "border-moss-700 bg-moss-50"
                      : "border-border bg-background-elevated hover:border-border-strong"
                  } disabled:cursor-not-allowed disabled:opacity-60`}
                >
                  <div className="pointer-events-none">
                    <StorefrontLayoutThumbnail theme={cardTheme} name={store.name} />
                  </div>
                  <p className="mt-3 text-sm font-semibold text-foreground">
                    {option.label}
                  </p>
                  <p className="mt-1 text-xs leading-5 text-foreground-secondary">
                    {option.description}
                  </p>
                </button>
              );
            })}
          </div>
        </div>
      </section>

      {error && (
        <p role="alert" className="text-sm text-danger">
          {error}
        </p>
      )}
      {success && (
        <p role="status" className="text-sm text-moss-700">
          Storefront theme saved.
        </p>
      )}

      <div className="flex items-center justify-end gap-3 border-t border-border-subtle pt-6">
        <button
          type="button"
          disabled={!dirty || pending}
          onClick={() => {
            setTheme(initialTheme);
            setError(null);
            setSuccess(false);
          }}
          className="inline-flex h-11 items-center px-4 text-sm font-medium text-foreground-secondary hover:text-foreground disabled:cursor-not-allowed disabled:opacity-50"
        >
          Reset
        </button>
        <button
          type="button"
          onClick={() => setPreviewOpen(true)}
          className="inline-flex h-11 items-center gap-2 rounded-md border border-border px-4 text-sm font-medium text-foreground hover:border-[color:var(--moss-700)]"
        >
          Preview
        </button>
        <button
          type="submit"
          data-testid="save-storefront-theme"
          disabled={!editable || !dirty || pending}
          className="inline-flex h-12 items-center justify-center rounded-md bg-primary px-6 text-base font-medium text-primary-foreground hover:bg-primary-hover disabled:cursor-not-allowed disabled:bg-ink-600"
        >
          {pending ? "Saving…" : "Save storefront theme"}
        </button>
      </div>

      {previewOpen ? (
        <PreviewModal
          theme={theme}
          name={store.name}
          slug={store.slug}
          onClose={() => setPreviewOpen(false)}
        />
      ) : null}
    </form>
  );
}

function PreviewModal({
  theme,
  name,
  slug,
  onClose,
}: {
  theme: StorefrontTheme;
  name: string;
  slug: string;
  onClose: () => void;
}) {
  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label="Storefront preview"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 sm:p-8"
      onClick={onClose}
    >
      <div
        className="w-full max-w-5xl overflow-hidden rounded-lg bg-[color:var(--background-elevated)] shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-border-subtle px-5 py-3">
          <p className="text-sm font-medium text-foreground">Storefront preview</p>
          <button
            type="button"
            onClick={onClose}
            className="text-sm text-foreground-secondary hover:text-foreground"
          >
            Close ✕
          </button>
        </div>
        <div className="max-h-[80vh] overflow-y-auto p-6">
          <StorefrontLayoutPreview theme={theme} name={name} slug={slug} />
        </div>
      </div>
    </div>
  );
}

function ModeToggle({
  value,
  disabled,
  onChange,
}: {
  value: StorefrontMode;
  disabled?: boolean;
  onChange: (value: StorefrontMode) => void;
}) {
  const options: Array<{ value: StorefrontMode; label: string }> = [
    { value: "light", label: "Light" },
    { value: "dark", label: "Dark" },
  ];
  return (
    <div className="inline-flex rounded-md border border-border bg-background-elevated p-1">
      {options.map((o) => {
        const active = value === o.value;
        return (
          <button
            key={o.value}
            type="button"
            disabled={disabled}
            aria-pressed={active}
            onClick={() => onChange(o.value)}
            className={`min-w-[88px] rounded-sm px-4 py-2 text-sm font-medium transition-colors ${
              active
                ? "bg-moss-700 text-white"
                : "text-foreground-secondary hover:text-foreground"
            } disabled:cursor-not-allowed disabled:opacity-60`}
          >
            {o.label}
          </button>
        );
      })}
    </div>
  );
}

function PaletteChip({
  option,
  active,
  mode,
  disabled,
  onClick,
}: {
  option: { value: StorefrontPalette; label: string; description: string };
  active: boolean;
  mode: StorefrontMode;
  disabled?: boolean;
  onClick: () => void;
}) {
  // Placeholder theme used just to extract the accent dot for this
  // palette chip — avoids spinning up a full resolved preview per chip.
  // We construct a tiny theme with this palette at the current mode,
  // read theme.colors.accent, and show it as a swatch.
  const swatch = useMemo(() => {
    const probe = normalizeStorefrontTheme({
      mode,
      palette: option.value,
      background: mode === "dark" ? "obsidian" : "white",
      layout: "editorial",
    });
    return probe.colors.accent;
  }, [mode, option.value]);

  return (
    <button
      type="button"
      disabled={disabled}
      aria-pressed={active}
      onClick={onClick}
      title={option.description}
      className={`inline-flex min-h-[44px] items-center gap-2 rounded-md border px-3 text-sm transition-colors ${
        active
          ? "border-moss-700 bg-moss-50 text-moss-700"
          : "border-border bg-background-elevated text-foreground-secondary hover:text-foreground"
      } disabled:cursor-not-allowed disabled:opacity-60`}
    >
      <span
        aria-hidden
        className="inline-block h-3.5 w-3.5 rounded-full border border-[color:var(--ink-900)]/15"
        style={{ backgroundColor: swatch }}
      />
      {option.label}
    </button>
  );
}

function ColorField({
  label,
  value,
  disabled,
  onChange,
}: {
  label: string;
  value: string;
  disabled?: boolean;
  onChange: (value: string) => void;
}) {
  const id = `color-${label.toLowerCase().replace(/\s+/g, "-")}`;
  return (
    <Field id={id} label={label}>
      <div className="flex items-center gap-3 rounded-md border border-border bg-background-elevated px-3 py-2">
        <input
          id={id}
          type="color"
          value={value}
          disabled={disabled}
          onChange={(e) => onChange(e.target.value)}
          className="h-10 w-10 rounded-sm border-0 bg-transparent p-0"
        />
        <span className="font-mono text-sm text-foreground">{value}</span>
      </div>
    </Field>
  );
}

function SelectField({
  id,
  label,
  value,
  options,
  disabled,
  onChange,
}: {
  id: string;
  label: string;
  value: string;
  options: Array<{ value: string; label: string }>;
  disabled?: boolean;
  onChange: (value: string) => void;
}) {
  return (
    <Field id={id} label={label}>
      <Select value={value} onValueChange={onChange} disabled={disabled}>
        <SelectTrigger id={id}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </Field>
  );
}

export function initialStorefrontTheme(store: Store): StorefrontTheme {
  return normalizeStorefrontTheme(
    store.storefront_theme ?? defaultStorefrontTheme,
  );
}

// Silence unused-type-import warnings for types that aren't referenced
// in the body after this refactor but are part of the public surface.
// These are imported for downstream consumers; keep them in scope.
export type { StorefrontBackground };
