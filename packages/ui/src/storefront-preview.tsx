import type { CSSProperties } from "react";

import { themeCssVariables, type StorefrontTheme } from "./storefront-theme";

/**
 * StorefrontLayoutPreview — shared mini schematic.
 *
 * Consumed by the admin StorefrontThemeForm so merchants see what
 * each layout actually looks like, and by any future preview widget
 * that needs a tiny accurate thumbnail. Every variant is a real
 * structural sketch using the theme tokens — not a generic card.
 *
 * The sketches are deliberately low-fidelity (blocks instead of
 * real copy) so they read as structure, not content. When a merchant
 * picks `editorial`, they see an editorial layout; when they pick
 * `compact`, they see a sidebar + 6-grid.
 */
export interface StorefrontLayoutPreviewProps {
  theme: StorefrontTheme;
  name: string;
  slug: string;
}

export function StorefrontLayoutPreview({
  theme,
  name,
  slug,
}: StorefrontLayoutPreviewProps) {
  const radius =
    theme.radius === "sharp"
      ? "6px"
      : theme.radius === "rounded"
        ? "16px"
        : "10px";

  // Spread the shared theme CSS vars (heading/body font, radius, density,
  // colors) onto the preview root so every descendant that reads
  // var(--storefront-heading-font) etc. picks up the merchant's choices.
  // Without this the preview used the browser's default font regardless
  // of the typography selectors above.
  const themeVars = themeCssVariables(theme) as CSSProperties;

  return (
    <div
      className="overflow-hidden border"
      style={{
        ...themeVars,
        background: theme.colors.background,
        color: theme.colors.text,
        borderColor: `${theme.colors.primary}22`,
        borderRadius: radius,
        fontFamily: "var(--storefront-body-font)",
      }}
    >
      <BrowserChrome theme={theme} slug={slug} />
      <div className="px-5 py-5">
        <LayoutSchematic theme={theme} name={name} />
      </div>
    </div>
  );
}

function BrowserChrome({
  theme,
  slug,
}: {
  theme: StorefrontTheme;
  slug: string;
}) {
  return (
    <div
      className="flex items-center gap-2 border-b px-4 py-3"
      style={{
        borderColor: `${theme.colors.primary}18`,
        background: `${theme.colors.surface}66`,
      }}
    >
      <span
        className="h-2 w-2 rounded-full"
        style={{ background: `${theme.colors.primary}33` }}
      />
      <span
        className="h-2 w-2 rounded-full"
        style={{ background: `${theme.colors.primary}33` }}
      />
      <span
        className="h-2 w-2 rounded-full"
        style={{ background: `${theme.colors.primary}33` }}
      />
      <span
        className="ml-3 text-[10px] font-semibold uppercase tracking-[0.18em]"
        style={{ color: `${theme.colors.primary}AA` }}
      >
        {slug}.mark8ly.com
      </span>
    </div>
  );
}

function LayoutSchematic({
  theme,
  name,
}: {
  theme: StorefrontTheme;
  name: string;
}) {
  switch (theme.layout) {
    case "classic-shop":
      return <ClassicSchematic theme={theme} name={name} />;
    case "split-hero":
      return <SplitHeroSchematic theme={theme} name={name} />;
    case "catalog-first":
      return <CatalogFirstSchematic theme={theme} name={name} />;
    case "story-led":
      return <StoryLedSchematic theme={theme} name={name} />;
    case "minimal":
      return <MinimalSchematic theme={theme} name={name} />;
    case "bold-promo":
      return <BoldPromoSchematic theme={theme} name={name} />;
    case "compact":
      return <CompactSchematic theme={theme} />;
    case "editorial":
    default:
      return <EditorialSchematic theme={theme} name={name} />;
  }
}

/* ============================================================
   Per-layout schematics — tiny structural sketches.
   Each one should be uniquely identifiable at a glance.
   ============================================================ */

function EditorialSchematic({
  theme,
  name,
}: {
  theme: StorefrontTheme;
  name: string;
}) {
  return (
    <div className="space-y-4">
      <Eyebrow theme={theme}>Issue 01</Eyebrow>
      <div className="grid grid-cols-12 items-end gap-3">
        <div className="col-span-7 space-y-2">
          <HeroHeadline theme={theme} name={name} size="lg" />
          <Bar theme={theme} width="80%" />
          <Bar theme={theme} width="60%" />
          <div className="pt-1">
            <ButtonBlock theme={theme} label="Shop the edit" />
          </div>
        </div>
        <div className="col-span-5">
          <Tile theme={theme} ratio="85%" />
        </div>
      </div>
      <MarqueeBar theme={theme} />
      <div className="grid grid-cols-3 gap-2">
        <Tile theme={theme} ratio="100%" small />
        <Tile theme={theme} ratio="100%" small />
        <Tile theme={theme} ratio="100%" small />
      </div>
    </div>
  );
}

function ClassicSchematic({
  theme,
  name,
}: {
  theme: StorefrontTheme;
  name: string;
}) {
  return (
    <div className="space-y-4">
      <div
        className="flex gap-3 border-b pb-2 text-[8px] font-semibold uppercase tracking-[0.18em]"
        style={{
          borderColor: `${theme.colors.primary}22`,
          color: `${theme.colors.primary}99`,
        }}
      >
        {["All", "New", "Best", "Gifts", "Sale"].map((c) => (
          <span key={c}>{c}</span>
        ))}
      </div>
      <div className="space-y-2 py-2 text-center">
        <HeroHeadline theme={theme} name={name} centered />
        <Bar theme={theme} width="70%" centered />
        <div className="flex justify-center">
          <ButtonBlock theme={theme} label="Shop" />
        </div>
      </div>
      <div className="grid grid-cols-4 gap-2">
        <Tile theme={theme} ratio="100%" small />
        <Tile theme={theme} ratio="100%" small />
        <Tile theme={theme} ratio="100%" small />
        <Tile theme={theme} ratio="100%" small />
      </div>
    </div>
  );
}

function SplitHeroSchematic({
  theme,
  name,
}: {
  theme: StorefrontTheme;
  name: string;
}) {
  return (
    <div className="space-y-4">
      <div
        className="grid grid-cols-2 gap-0 overflow-hidden"
        style={{
          borderRadius: "var(--storefront-radius)",
          background: `${theme.colors.surface}66`,
        }}
      >
        <div className="space-y-2 p-4">
          <Eyebrow theme={theme}>Featured</Eyebrow>
          <HeroHeadline theme={theme} name={name} />
          <Bar theme={theme} width="80%" />
          <div className="pt-1">
            <ButtonBlock theme={theme} label="Shop" />
          </div>
        </div>
        <div className="relative">
          <Tile theme={theme} ratio="100%" fill />
        </div>
      </div>
      <div className="grid grid-cols-3 gap-2">
        <Tile theme={theme} ratio="100%" small />
        <Tile theme={theme} ratio="100%" small />
        <Tile theme={theme} ratio="100%" small />
      </div>
    </div>
  );
}

function CatalogFirstSchematic({
  theme,
  name,
}: {
  theme: StorefrontTheme;
  name: string;
}) {
  return (
    <div className="space-y-4">
      <div className="flex items-end justify-between">
        <HeroHeadline theme={theme} name={name} size="sm" />
        <div className="flex gap-1">
          {[1, 2, 3].map((i) => (
            <span
              key={i}
              className="h-3 w-8 border"
              style={{
                borderColor: `${theme.colors.primary}33`,
                borderRadius: "var(--storefront-radius)",
              }}
            />
          ))}
        </div>
      </div>
      <div className="grid grid-cols-4 grid-rows-2 gap-2">
        <div className="col-span-2 row-span-2">
          <Tile theme={theme} ratio="100%" fill />
        </div>
        <Tile theme={theme} ratio="100%" small />
        <Tile theme={theme} ratio="100%" small />
        <Tile theme={theme} ratio="100%" small />
        <Tile theme={theme} ratio="100%" small />
      </div>
    </div>
  );
}

function StoryLedSchematic({
  theme,
  name,
}: {
  theme: StorefrontTheme;
  name: string;
}) {
  return (
    <div className="mx-auto max-w-[85%] space-y-3">
      <Eyebrow theme={theme}>A story</Eyebrow>
      <HeroHeadline theme={theme} name={name} />
      <Bar theme={theme} width="95%" />
      <Bar theme={theme} width="88%" />
      <div className="grid grid-cols-[40%_1fr] gap-3 pt-1">
        <Tile theme={theme} ratio="120%" />
        <div className="space-y-1.5">
          <Bar theme={theme} width="100%" />
          <Bar theme={theme} width="92%" />
          <Bar theme={theme} width="96%" />
          <Bar theme={theme} width="80%" />
        </div>
      </div>
      <div
        className="border-l-2 pl-3 text-[9px] italic"
        style={{
          borderColor: theme.colors.accent,
          color: `${theme.colors.text}CC`,
        }}
      >
        &ldquo;A pull quote from the founder.&rdquo;
      </div>
    </div>
  );
}

function MinimalSchematic({
  theme,
  name,
}: {
  theme: StorefrontTheme;
  name: string;
}) {
  return (
    <div className="space-y-6 py-4 text-center">
      <HeroHeadline theme={theme} name={name} centered />
      <div className="mx-auto w-3/4">
        <Tile theme={theme} ratio="55%" />
      </div>
      <div className="flex justify-center">
        <ButtonBlock theme={theme} label="Enter the store" />
      </div>
    </div>
  );
}

function BoldPromoSchematic({
  theme,
  name,
}: {
  theme: StorefrontTheme;
  name: string;
}) {
  return (
    <div className="space-y-4">
      <div
        className="p-4"
        style={{
          background: `linear-gradient(135deg, ${theme.colors.primary}, ${theme.colors.accent})`,
          color: "#fff",
          borderRadius: "var(--storefront-radius)",
        }}
      >
        <div className="grid grid-cols-12 gap-3">
          <div className="col-span-7 space-y-1.5">
            <p className="text-[8px] font-semibold uppercase tracking-[0.2em] text-white/70">
              The drop
            </p>
            <p
              className="text-xl font-medium leading-none tracking-tight"
              style={{ fontFamily: "var(--storefront-heading-font)" }}
            >
              {name}
            </p>
            <div className="pt-1">
              <span
                className="inline-flex h-5 items-center bg-white px-3 text-[8px] font-semibold uppercase tracking-[0.14em] text-neutral-900"
                style={{ borderRadius: "var(--storefront-radius)" }}
              >
                Shop drop
              </span>
            </div>
          </div>
          <div className="col-span-5 grid grid-cols-3 gap-2 self-end">
            {["12", "01", "∞"].map((n) => (
              <p
                key={n}
                className="text-2xl font-medium leading-none"
                style={{ fontFamily: "var(--storefront-heading-font)" }}
              >
                {n}
              </p>
            ))}
          </div>
        </div>
      </div>
      <div className="grid grid-cols-3 gap-2">
        <Tile theme={theme} ratio="100%" small />
        <Tile theme={theme} ratio="100%" small />
        <Tile theme={theme} ratio="100%" small />
      </div>
    </div>
  );
}

function CompactSchematic({ theme }: { theme: StorefrontTheme }) {
  return (
    <div className="grid grid-cols-[28%_1fr] gap-3">
      <aside className="space-y-1">
        <div
          className="h-2 w-3/4 rounded-sm"
          style={{ background: `${theme.colors.primary}55` }}
        />
        <div
          className="h-1.5 w-1/2 rounded-sm"
          style={{ background: `${theme.colors.primary}22` }}
        />
        <div className="pt-2 space-y-1">
          {[1, 2, 3, 4, 5].map((i) => (
            <div
              key={i}
              className="h-1 rounded-sm"
              style={{
                width: `${70 - i * 5}%`,
                background: `${theme.colors.primary}22`,
              }}
            />
          ))}
        </div>
      </aside>
      <div className="grid grid-cols-3 gap-2">
        {Array.from({ length: 6 }).map((_, i) => (
          <Tile key={i} theme={theme} ratio="100%" small />
        ))}
      </div>
    </div>
  );
}

/* ============================================================
   Low-level visual primitives
   ============================================================ */

function Eyebrow({
  theme,
  children,
}: {
  theme: StorefrontTheme;
  children: React.ReactNode;
}) {
  return (
    <p
      className="text-[8px] font-semibold uppercase tracking-[0.24em]"
      style={{ color: `${theme.colors.accent}D9` }}
    >
      {children}
    </p>
  );
}

function HeroHeadline({
  theme,
  name,
  size = "md",
  centered = false,
}: {
  theme: StorefrontTheme;
  name: string;
  size?: "sm" | "md" | "lg";
  centered?: boolean;
}) {
  const cls =
    size === "lg"
      ? "text-3xl"
      : size === "sm"
        ? "text-lg"
        : "text-2xl";
  return (
    <p
      className={`${cls} font-medium leading-[0.95] tracking-[-0.02em] ${
        centered ? "text-center" : ""
      }`}
      style={{
        fontFamily: "var(--storefront-heading-font)",
        color: theme.colors.text,
      }}
    >
      {name}
    </p>
  );
}

function MarqueeBar({ theme }: { theme: StorefrontTheme }) {
  return (
    <div
      className="overflow-hidden border-y py-1.5 text-[8px] font-semibold uppercase tracking-[0.28em]"
      style={{
        borderColor: `${theme.colors.primary}22`,
        background: `${theme.colors.surface}88`,
        color: `${theme.colors.primary}99`,
      }}
    >
      hand picked · small batch · ships worldwide · est. 2026 · hand picked ·
      small batch
    </div>
  );
}

function Bar({
  theme,
  width,
  centered,
}: {
  theme: StorefrontTheme;
  width: string;
  centered?: boolean;
}) {
  return (
    <div
      className={`h-1.5 rounded-sm ${centered ? "mx-auto" : ""}`}
      style={{ width, background: `${theme.colors.text}22` }}
    />
  );
}

function ButtonBlock({
  theme,
  label,
}: {
  theme: StorefrontTheme;
  label: string;
}) {
  return (
    <span
      className="inline-flex h-6 items-center px-3 text-[9px] font-semibold uppercase tracking-[0.14em] text-white"
      style={{
        background: theme.colors.primary,
        borderRadius: "var(--storefront-radius)",
      }}
    >
      {label}
    </span>
  );
}

function Tile({
  theme,
  ratio,
  fill = false,
}: {
  theme: StorefrontTheme;
  ratio: string;
  small?: boolean;
  fill?: boolean;
}) {
  const style: CSSProperties = fill
    ? {
        background: `${theme.colors.primary}10`,
        border: `1px solid ${theme.colors.primary}22`,
        borderRadius: "var(--storefront-radius)",
        height: "100%",
        minHeight: "100%",
      }
    : {
        background: `${theme.colors.primary}10`,
        border: `1px solid ${theme.colors.primary}22`,
        borderRadius: "var(--storefront-radius)",
        paddingTop: ratio,
      };
  return <div style={style} />;
}
