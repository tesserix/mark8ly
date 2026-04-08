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
import { StorefrontLayoutPreview } from "@repo/ui/storefront-preview";

import { updateStorefrontTheme } from "@/app/settings/storefront/actions";
import type { Store } from "@/lib/api/platform-api";
import {
  defaultStorefrontTheme,
  normalizeStorefrontTheme,
  storefrontFontOptions,
  storefrontLayoutOptions,
  storefrontPresetOptions,
  withPresetColors,
  type StorefrontDensity,
  type StorefrontFont,
  type StorefrontMotion,
  type StorefrontPreset,
  type StorefrontRadius,
  type StorefrontTheme,
} from "@repo/ui/storefront-theme";

interface StorefrontThemeFormProps {
  store: Store;
  editable?: boolean;
}

/**
 * StorefrontThemeForm — the merchant-facing editor for the storefront
 * theme system. Layout selection, preset, color overrides, typography,
 * motion, density, radius. Editorial Paper · Ink · Moss surface — no
 * glassmorphism, no card chrome around individual controls, hairline
 * rules between sections.
 *
 * State stays simple useState — this is a single-screen editor with no
 * cross-field validation. The shape is enforced by the typed
 * StorefrontTheme interface from @repo/ui/storefront-theme.
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
  const [pending, startTransition] = useTransition();

  const dirty = JSON.stringify(theme) !== JSON.stringify(initialTheme);

  function applyPreset(preset: StorefrontPreset) {
    setTheme((current) => withPresetColors({ ...current }, preset));
    setSuccess(false);
  }

  function updateTheme(next: StorefrontTheme) {
    setTheme(next);
    setSuccess(false);
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!dirty) return;
    setError(null);
    setSuccess(false);

    startTransition(async () => {
      const result = await updateStorefrontTheme(theme);
      if (!result.ok) {
        setError(result.message);
        return;
      }
      setSuccess(true);
    });
  }

  const disabled = !editable || pending;

  return (
    <form onSubmit={handleSubmit} className="space-y-12">
      {/* Layout */}
      <section className="space-y-5 border-t border-border-subtle pt-10">
        <div className="space-y-2">
          <p className="eyebrow">Layout</p>
          <h2 className="font-serif text-2xl font-medium tracking-tight text-foreground">
            Choose a structure
          </h2>
          <p className="max-w-2xl text-sm leading-7 text-foreground-secondary">
            Pick a storefront structure first, then fine-tune the styling.
            Layout changes affect the overall composition customers see when
            they land on your store.
          </p>
        </div>

        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
          {storefrontLayoutOptions.map((option) => {
            const active = theme.layout === option.value;
            return (
              <button
                key={option.value}
                type="button"
                data-testid={`layout-${option.value}`}
                aria-pressed={active}
                disabled={disabled}
                onClick={() => updateTheme({ ...theme, layout: option.value })}
                className={`min-h-[44px] rounded-md border px-4 py-4 text-left transition-colors ${
                  active
                    ? "border-moss-700 bg-moss-50"
                    : "border-border bg-background-elevated hover:border-border-strong"
                } disabled:cursor-not-allowed disabled:opacity-60`}
              >
                <p className="text-sm font-semibold text-foreground">
                  {option.label}
                </p>
                <p className="mt-2 text-xs leading-5 text-foreground-secondary">
                  {option.description}
                </p>
              </button>
            );
          })}
        </div>
      </section>

      {/* Style + preview */}
      <section className="grid gap-10 border-t border-border-subtle pt-10 xl:grid-cols-[minmax(0,1.15fr)_minmax(20rem,0.85fr)]">
        <div className="space-y-8">
          <div className="space-y-2">
            <p className="eyebrow">Style</p>
            <h2 className="font-serif text-2xl font-medium tracking-tight text-foreground">
              Brand controls
            </h2>
            <p className="max-w-xl text-sm leading-7 text-foreground-secondary">
              Start from a curated preset, then customize colors, typography,
              motion, and density.
            </p>
          </div>

          <Field id="preset" label="Preset">
            <div className="flex flex-wrap gap-2 pt-1">
              {storefrontPresetOptions.map((option) => {
                const active = theme.preset === option.value;
                return (
                  <button
                    key={option.value}
                    type="button"
                    disabled={disabled}
                    onClick={() => applyPreset(option.value)}
                    className={`min-h-[44px] rounded-md border px-4 text-sm transition-colors ${
                      active
                        ? "border-moss-700 bg-moss-50 text-moss-700"
                        : "border-border bg-background-elevated text-foreground-secondary hover:text-foreground"
                    } disabled:cursor-not-allowed disabled:opacity-60`}
                  >
                    {option.label}
                  </button>
                );
              })}
            </div>
          </Field>

          <div className="grid gap-5 sm:grid-cols-2">
            <ColorField
              label="Primary"
              value={theme.colors.primary}
              disabled={disabled}
              onChange={(value) =>
                updateTheme({
                  ...theme,
                  colors: { ...theme.colors, primary: value },
                })
              }
            />
            <ColorField
              label="Accent"
              value={theme.colors.accent}
              disabled={disabled}
              onChange={(value) =>
                updateTheme({
                  ...theme,
                  colors: { ...theme.colors, accent: value },
                })
              }
            />
            <ColorField
              label="Background"
              value={theme.colors.background}
              disabled={disabled}
              onChange={(value) =>
                updateTheme({
                  ...theme,
                  colors: { ...theme.colors, background: value },
                })
              }
            />
            <ColorField
              label="Surface"
              value={theme.colors.surface}
              disabled={disabled}
              onChange={(value) =>
                updateTheme({
                  ...theme,
                  colors: { ...theme.colors, surface: value },
                })
              }
            />
            <ColorField
              label="Text"
              value={theme.colors.text}
              disabled={disabled}
              onChange={(value) =>
                updateTheme({
                  ...theme,
                  colors: { ...theme.colors, text: value },
                })
              }
            />
          </div>

          <div className="grid gap-5 sm:grid-cols-2">
            <SelectField
              id="heading-font"
              label="Heading font"
              value={theme.typography.headingFont}
              options={storefrontFontOptions}
              disabled={disabled}
              onChange={(value) =>
                updateTheme({
                  ...theme,
                  typography: {
                    ...theme.typography,
                    headingFont: value as StorefrontFont,
                  },
                })
              }
            />
            <SelectField
              id="body-font"
              label="Body font"
              value={theme.typography.bodyFont}
              options={storefrontFontOptions}
              disabled={disabled}
              onChange={(value) =>
                updateTheme({
                  ...theme,
                  typography: {
                    ...theme.typography,
                    bodyFont: value as StorefrontFont,
                  },
                })
              }
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
              onChange={(value) =>
                updateTheme({ ...theme, motion: value as StorefrontMotion })
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
              onChange={(value) =>
                updateTheme({ ...theme, density: value as StorefrontDensity })
              }
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
              onChange={(value) =>
                updateTheme({ ...theme, radius: value as StorefrontRadius })
              }
            />
          </div>
        </div>

        <aside className="space-y-4">
          <div className="space-y-2">
            <p className="eyebrow">Preview</p>
            <h2 className="font-serif text-2xl font-medium tracking-tight text-foreground">
              How it reads
            </h2>
            <p className="text-sm leading-7 text-foreground-secondary">
              A compact approximation of the storefront mood. The live store
              renders the full layout.
            </p>
          </div>

          <StorefrontLayoutPreview
            theme={theme}
            name={store.name}
            slug={store.slug}
          />
        </aside>
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
          type="submit"
          data-testid="save-storefront-theme"
          disabled={!editable || !dirty || pending}
          className="inline-flex h-12 items-center justify-center rounded-md bg-primary px-6 text-base font-medium text-primary-foreground hover:bg-primary-hover disabled:cursor-not-allowed disabled:bg-ink-600"
        >
          {pending ? "Saving…" : "Save storefront theme"}
        </button>
      </div>
    </form>
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
