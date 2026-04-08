import type { CSSProperties, ReactNode } from "react";

import type { PublicStore } from "@/lib/api/platform-api";
import type { StorefrontTheme } from "@repo/ui/storefront-theme";

/* ============================================================
   Shared layout primitives
   ------------------------------------------------------------
   Every storefront layout consumes the same theme tokens and
   uses the same primitive set. A layout switch + a preset swap
   should produce a wildly different look without any layout
   touching brand colors directly.
   ============================================================ */

export interface LayoutProps {
  store: PublicStore;
  theme: StorefrontTheme;
}

export function HeroTitle({
  store,
  theme,
  align = "left",
}: {
  store: PublicStore;
  theme: StorefrontTheme;
  align?: "left" | "center";
}) {
  return (
    <div className={align === "center" ? "text-center" : ""}>
      <h1
        className="text-5xl font-medium tracking-tight sm:text-6xl"
        style={headingStyle()}
      >
        {store.name}
      </h1>
      <p className="mt-4 text-base leading-7 opacity-75">
        Theme preset: {theme.preset.replace("-", " ")} ·{" "}
        {theme.layout.replace("-", " ")}
      </p>
    </div>
  );
}

export function ActionRow({ theme }: { theme: StorefrontTheme }) {
  return (
    <div className="flex flex-wrap gap-3">
      <PrimaryButton theme={theme}>Browse the store</PrimaryButton>
      <SecondaryButton theme={theme}>See featured collections</SecondaryButton>
    </div>
  );
}

export function CenteredCta({ theme }: { theme: StorefrontTheme }) {
  return (
    <div className="flex justify-center">
      <PrimaryButton theme={theme}>Start browsing</PrimaryButton>
    </div>
  );
}

export function Card({
  theme,
  title,
  body,
  compact = false,
  children,
}: {
  theme: StorefrontTheme;
  title: string;
  body: string;
  compact?: boolean;
  children?: ReactNode;
}) {
  return (
    <section
      className={compact ? "space-y-3 p-5" : "space-y-4 p-6"}
      style={surfaceStyle(theme)}
    >
      <h2 className="text-lg font-semibold" style={headingStyle()}>
        {title}
      </h2>
      <p className="text-sm leading-6 opacity-75">{body}</p>
      {children}
    </section>
  );
}

export function StoryPanel({
  theme,
  eyebrow,
  title,
  body,
  large = false,
}: {
  theme: StorefrontTheme;
  eyebrow: string;
  title: string;
  body: string;
  large?: boolean;
}) {
  return (
    <section
      className={large ? "space-y-5 p-8" : "space-y-4 p-6"}
      style={surfaceStyle(theme)}
    >
      <p
        className="text-[11px] font-semibold uppercase tracking-[0.24em]"
        style={{ color: `${theme.colors.primary}CC` }}
      >
        {eyebrow}
      </p>
      <h2
        className={
          large
            ? "text-4xl font-medium tracking-tight"
            : "text-2xl font-medium tracking-tight"
        }
        style={headingStyle()}
      >
        {title}
      </h2>
      <p className="text-sm leading-7 opacity-75">{body}</p>
    </section>
  );
}

export function MiniStat({
  theme,
  label,
  value,
}: {
  theme: StorefrontTheme;
  label: string;
  value: string;
}) {
  return (
    <div
      className="rounded-[1rem] border px-4 py-4"
      style={{
        background: `${theme.colors.background}99`,
        borderColor: `${theme.colors.primary}22`,
      }}
    >
      <div className="text-[11px] font-semibold uppercase tracking-[0.18em] opacity-55">
        {label}
      </div>
      <div className="mt-2 text-base font-semibold">{value}</div>
    </div>
  );
}

export function PrimaryButton({
  theme,
  children,
}: {
  theme: StorefrontTheme;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      className="rounded-full px-5 py-3 text-sm font-semibold text-white"
      style={{
        background: theme.colors.primary,
        boxShadow:
          theme.motion === "none"
            ? "none"
            : `0 18px 32px ${theme.colors.primary}30`,
      }}
    >
      {children}
    </button>
  );
}

export function SecondaryButton({
  theme,
  children,
}: {
  theme: StorefrontTheme;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      className="rounded-full border px-5 py-3 text-sm font-semibold"
      style={{
        borderColor: `${theme.colors.primary}33`,
        background: `${theme.colors.surface}D9`,
        color: theme.colors.text,
      }}
    >
      {children}
    </button>
  );
}

export function PromoButton({ children }: { children: ReactNode }) {
  return (
    <button
      type="button"
      className="rounded-full bg-white px-5 py-3 text-sm font-semibold text-neutral-900"
    >
      {children}
    </button>
  );
}

export function PromoGhostButton({ children }: { children: ReactNode }) {
  return (
    <button
      type="button"
      className="rounded-full border border-white/35 px-5 py-3 text-sm font-semibold text-white"
    >
      {children}
    </button>
  );
}

export function headingStyle(): CSSProperties {
  return { fontFamily: "var(--store-heading-font)" };
}

export function surfaceStyle(theme: StorefrontTheme): CSSProperties {
  return {
    background: `${theme.colors.surface}F0`,
    border: `1px solid ${theme.colors.primary}22`,
    borderRadius: "var(--store-radius)",
    boxShadow:
      theme.motion === "none"
        ? "none"
        : `0 18px 40px ${theme.colors.primary}12`,
  };
}
