"use client";

import { useMemo, useState, useTransition } from "react";
import {
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@tesserix/web";

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

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      <section className="admin-panel space-y-6 rounded-[1.8rem] p-6">
        <div className="space-y-2">
          <h2 className="text-sm font-semibold uppercase tracking-[0.16em] text-muted-foreground">
            Layout presets
          </h2>
          <p className="max-w-3xl text-sm leading-6 text-muted-foreground">
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
                disabled={!editable || pending}
                onClick={() => updateTheme({ ...theme, layout: option.value })}
                className={`rounded-[1.35rem] border px-4 py-4 text-left transition-[transform,border-color,box-shadow,background-color] ${
                  active
                    ? "border-[var(--app-orange)] bg-[rgba(230,122,47,0.08)] shadow-[0_14px_34px_rgba(76,52,24,0.08)]"
                    : "border-border/70 bg-white/72 hover:-translate-y-0.5 hover:border-[rgba(138,100,64,0.35)]"
                } disabled:cursor-not-allowed disabled:opacity-60`}
              >
                <p className="text-sm font-semibold text-foreground">{option.label}</p>
                <p className="mt-2 text-xs leading-5 text-muted-foreground">
                  {option.description}
                </p>
              </button>
            );
          })}
        </div>
      </section>

      <section className="grid gap-6 xl:grid-cols-[minmax(0,1.15fr)_minmax(20rem,0.85fr)]">
        <div className="admin-panel space-y-6 rounded-[1.8rem] p-6">
          <div className="space-y-2">
            <h2 className="text-sm font-semibold uppercase tracking-[0.16em] text-muted-foreground">
              Style controls
            </h2>
            <p className="text-sm leading-6 text-muted-foreground">
              Start from a curated preset, then customize brand colors,
              typography, motion, and density.
            </p>
          </div>

          <div className="space-y-3">
            <Label>Preset</Label>
            <div className="flex flex-wrap gap-2">
              {storefrontPresetOptions.map((option) => {
                const active = theme.preset === option.value;
                return (
                  <button
                    key={option.value}
                    type="button"
                    disabled={!editable || pending}
                    onClick={() => applyPreset(option.value)}
                    className={`rounded-full border px-3 py-2 text-sm transition-colors ${
                      active
                        ? "border-[var(--app-orange)] bg-[rgba(230,122,47,0.08)] text-[var(--app-orange)]"
                        : "border-border/70 bg-white/72 text-muted-foreground hover:text-foreground"
                    } disabled:cursor-not-allowed disabled:opacity-60`}
                  >
                    {option.label}
                  </button>
                );
              })}
            </div>
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <ColorField
              label="Primary"
              value={theme.colors.primary}
              disabled={!editable || pending}
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
              disabled={!editable || pending}
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
              disabled={!editable || pending}
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
              disabled={!editable || pending}
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
              disabled={!editable || pending}
              onChange={(value) =>
                updateTheme({
                  ...theme,
                  colors: { ...theme.colors, text: value },
                })
              }
            />
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <SelectField
              label="Heading font"
              value={theme.typography.headingFont}
              options={storefrontFontOptions}
              disabled={!editable || pending}
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
              label="Body font"
              value={theme.typography.bodyFont}
              options={storefrontFontOptions}
              disabled={!editable || pending}
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
              label="Motion"
              value={theme.motion}
              options={[
                { value: "none", label: "None" },
                { value: "subtle", label: "Subtle" },
                { value: "expressive", label: "Expressive" },
              ]}
              disabled={!editable || pending}
              onChange={(value) =>
                updateTheme({ ...theme, motion: value as StorefrontMotion })
              }
            />
            <SelectField
              label="Density"
              value={theme.density}
              options={[
                { value: "compact", label: "Compact" },
                { value: "balanced", label: "Balanced" },
                { value: "airy", label: "Airy" },
              ]}
              disabled={!editable || pending}
              onChange={(value) =>
                updateTheme({ ...theme, density: value as StorefrontDensity })
              }
            />
            <SelectField
              label="Corner style"
              value={theme.radius}
              options={[
                { value: "sharp", label: "Sharp" },
                { value: "soft", label: "Soft" },
                { value: "rounded", label: "Rounded" },
              ]}
              disabled={!editable || pending}
              onChange={(value) =>
                updateTheme({ ...theme, radius: value as StorefrontRadius })
              }
            />
          </div>
        </div>

        <section className="admin-panel space-y-5 rounded-[1.8rem] p-6">
          <div className="space-y-2">
            <h2 className="text-sm font-semibold uppercase tracking-[0.16em] text-muted-foreground">
              Preview
            </h2>
            <p className="text-sm leading-6 text-muted-foreground">
              A compact approximation of how the storefront mood will read to
              customers.
            </p>
          </div>

          <StorefrontPreview theme={theme} name={store.name} slug={store.slug} />
        </section>
      </section>

      {error && (
        <div
          role="alert"
          className="rounded-2xl border border-destructive/20 bg-destructive/5 px-4 py-3 text-sm text-destructive"
        >
          {error}
        </div>
      )}
      {success && (
        <div
          role="status"
          className="rounded-2xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-800"
        >
          Storefront theme saved.
        </div>
      )}

      <div className="flex items-center justify-end gap-3 border-t admin-soft-rule pt-6">
        <button
          type="button"
          disabled={!dirty || pending}
          onClick={() => {
            setTheme(initialTheme);
            setError(null);
            setSuccess(false);
          }}
          className="rounded-xl px-4 py-2.5 text-sm font-medium text-muted-foreground hover:text-foreground disabled:cursor-not-allowed disabled:opacity-50"
        >
          Reset
        </button>
        <button
          type="submit"
          data-testid="save-storefront-theme"
          disabled={!editable || !dirty || pending}
          className="inline-flex items-center justify-center rounded-xl bg-primary px-5 py-2.5 text-sm font-medium text-primary-foreground shadow-[0_14px_30px_rgba(31,30,28,0.18)] transition-[transform,box-shadow,opacity] hover:-translate-y-0.5 hover:opacity-95 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {pending ? "Saving..." : "Save storefront theme"}
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
  return (
    <div className="space-y-2">
      <Label>{label}</Label>
      <div className="flex items-center gap-3 rounded-[1rem] border border-border/70 bg-white/72 px-3 py-2">
        <input
          type="color"
          value={value}
          disabled={disabled}
          onChange={(e) => onChange(e.target.value)}
          className="h-10 w-10 rounded-md border-0 bg-transparent p-0"
        />
        <span className="text-sm font-medium text-foreground">{value}</span>
      </div>
    </div>
  );
}

function SelectField({
  label,
  value,
  options,
  disabled,
  onChange,
}: {
  label: string;
  value: string;
  options: Array<{ value: string; label: string }>;
  disabled?: boolean;
  onChange: (value: string) => void;
}) {
  return (
    <div className="space-y-2">
      <Label>{label}</Label>
      <Select value={value} onValueChange={onChange} disabled={disabled}>
        <SelectTrigger className="h-10 rounded-xl border-border bg-white/82 text-sm shadow-sm">
          <SelectValue />
        </SelectTrigger>
        <SelectContent className="rounded-2xl border-border/80 bg-[rgba(255,252,248,0.98)] shadow-[0_24px_60px_rgba(76,52,24,0.14)]">
          {options.map((option) => (
            <SelectItem key={option.value} value={option.value} className="rounded-xl">
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function StorefrontPreview({
  theme,
  name,
  slug,
}: {
  theme: StorefrontTheme;
  name: string;
  slug: string;
}) {
  const radius =
    theme.radius === "sharp" ? "1rem" : theme.radius === "rounded" ? "2rem" : "1.4rem";
  const spacing =
    theme.density === "compact" ? "1rem" : theme.density === "airy" ? "1.6rem" : "1.25rem";

  return (
    <div
      className="overflow-hidden border shadow-[0_22px_44px_rgba(76,52,24,0.08)]"
      style={{
        background: theme.colors.background,
        color: theme.colors.text,
        borderColor: `${theme.colors.primary}22`,
        borderRadius: radius,
      }}
    >
      <div
        className="border-b px-5 py-4 text-xs font-semibold uppercase tracking-[0.2em]"
        style={{
          borderColor: `${theme.colors.primary}22`,
          color: theme.colors.primary,
        }}
      >
        {slug}.mark8ly.com
      </div>
      <div className="space-y-4 px-5 py-5" style={{ gap: spacing }}>
        <div
          className="inline-flex items-center rounded-full px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.18em]"
          style={{
            background: `${theme.colors.accent}18`,
            color: theme.colors.accent,
          }}
        >
          {theme.layout.replace(/-/g, " ")}
        </div>
        <div className="space-y-3">
          <h3 className="text-3xl font-medium tracking-tight">{name}</h3>
          <p className="max-w-sm text-sm leading-6 opacity-75">
            Your storefront styling will carry this mood across the hero,
            supporting sections, and launch CTA.
          </p>
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          <div
            className="rounded-[1.15rem] border px-4 py-4"
            style={{
              background: theme.colors.surface,
              borderColor: `${theme.colors.primary}22`,
            }}
          >
            <div className="text-xs uppercase tracking-[0.18em] opacity-55">
              Primary action
            </div>
            <div
              className="mt-3 inline-flex rounded-full px-4 py-2 text-sm font-semibold text-white"
              style={{ background: theme.colors.primary }}
            >
              Shop the collection
            </div>
          </div>
          <div
            className="rounded-[1.15rem] border px-4 py-4"
            style={{
              background: theme.colors.surface,
              borderColor: `${theme.colors.primary}22`,
            }}
          >
            <div className="text-xs uppercase tracking-[0.18em] opacity-55">
              Styling notes
            </div>
            <p className="mt-3 text-sm opacity-75">
              {theme.motion} motion, {theme.density} spacing,{" "}
              {theme.typography.headingFont} headings.
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}

export function initialStorefrontTheme(store: Store): StorefrontTheme {
  return normalizeStorefrontTheme(store.storefront_theme ?? defaultStorefrontTheme);
}
