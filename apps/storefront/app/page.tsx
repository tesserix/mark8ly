import type { CSSProperties, ReactNode } from "react";
import { headers } from "next/headers";

import { fetchStoreBySlug, type PublicStore } from "@/lib/api/platform-api";
import {
  fontStacks,
  normalizeStorefrontTheme,
  themeRadius,
  themeSpacing,
  type StorefrontTheme,
} from "@/lib/storefront-theme";
import { slugFromHost } from "@/lib/slug";

export const dynamic = "force-dynamic";

interface PageProps {
  searchParams: Promise<{ slug?: string }>;
}

export default async function StoreHomePage({ searchParams }: PageProps) {
  const { slug: slugFromQuery } = await searchParams;
  const h = await headers();
  const host = h.get("host");

  const slug =
    slugFromQuery ||
    slugFromHost(host) ||
    process.env.DEFAULT_STORE_SLUG ||
    "";

  const store = slug ? await fetchStoreBySlug(slug) : null;

  if (!store) {
    return <StoreNotFound slug={slug} />;
  }

  return <StoreLanding store={store} />;
}

function StoreLanding({ store }: { store: PublicStore }) {
  const theme = normalizeStorefrontTheme(store.storefront_theme);
  const style = themeStyle(theme);

  return (
    <main
      className="min-h-screen"
      style={{
        background:
          theme.preset === "midnight"
            ? theme.colors.background
            : `radial-gradient(circle at top left, ${theme.colors.accent}18, transparent 24%), radial-gradient(circle at top right, ${theme.colors.primary}12, transparent 28%), ${theme.colors.background}`,
        color: theme.colors.text,
        fontFamily: "var(--store-body-font)",
        ...style,
      }}
    >
      <div className="mx-auto max-w-6xl px-6 py-8 sm:px-8">
        <TopBar store={store} theme={theme} />
        {renderLayout(store, theme)}
      </div>
    </main>
  );
}

function TopBar({
  store,
  theme,
}: {
  store: PublicStore;
  theme: StorefrontTheme;
}) {
  return (
    <header className="mb-10 flex flex-wrap items-center justify-between gap-4">
      <div>
        <p
          className="text-[11px] font-semibold uppercase tracking-[0.24em]"
          style={{ color: `${theme.colors.primary}CC` }}
        >
          {store.slug}.mark8ly.com
        </p>
        <div className="mt-3 flex flex-wrap items-center gap-2">
          <MetaChip label="Currency" value={store.currency_code} theme={theme} />
          <MetaChip label="Country" value={store.country_code} theme={theme} />
          <MetaChip label="Timezone" value={store.timezone} theme={theme} />
        </div>
      </div>
      <div
        className="rounded-full border px-4 py-2 text-xs font-semibold uppercase tracking-[0.18em]"
        style={{
          borderColor: `${theme.colors.primary}33`,
          background: `${theme.colors.surface}CC`,
          color: theme.colors.primary,
        }}
      >
        Powered by mark8ly
      </div>
    </header>
  );
}

function renderLayout(store: PublicStore, theme: StorefrontTheme) {
  switch (theme.layout) {
    case "classic-shop":
      return <ClassicShopLayout store={store} theme={theme} />;
    case "split-hero":
      return <SplitHeroLayout store={store} theme={theme} />;
    case "catalog-first":
      return <CatalogFirstLayout store={store} theme={theme} />;
    case "story-led":
      return <StoryLedLayout store={store} theme={theme} />;
    case "minimal":
      return <MinimalLayout store={store} theme={theme} />;
    case "bold-promo":
      return <BoldPromoLayout store={store} theme={theme} />;
    case "compact":
      return <CompactLayout store={store} theme={theme} />;
    case "editorial":
    default:
      return <EditorialLayout store={store} theme={theme} />;
  }
}

function EditorialLayout({
  store,
  theme,
}: {
  store: PublicStore;
  theme: StorefrontTheme;
}) {
  return (
    <section className="grid gap-8 lg:grid-cols-[minmax(0,1.1fr)_minmax(18rem,0.9fr)] lg:items-end">
      <div className="space-y-6">
        <HeroTitle store={store} theme={theme} />
        <p className="max-w-2xl text-lg leading-8 opacity-80">
          A warmer, more authored storefront for merchants who want to lead
          with craft, clarity, and a memorable first impression.
        </p>
        <ActionRow theme={theme} />
      </div>
      <StoryPanel
        theme={theme}
        eyebrow="Launch mood"
        title="A storefront with room for story, products, and launch signals."
        body="Use this layout when your brand needs a richer opening frame before customers start browsing."
      />
    </section>
  );
}

function ClassicShopLayout({
  store,
  theme,
}: {
  store: PublicStore;
  theme: StorefrontTheme;
}) {
  return (
    <section className="space-y-8">
      <HeroTitle store={store} theme={theme} align="center" />
      <div className="mx-auto grid max-w-5xl gap-4 md:grid-cols-3">
        {["Ready to launch", "Clear merchandising", "Designed to convert"].map((item) => (
          <Card key={item} theme={theme} title={item} body="A balanced storefront shell that feels familiar without looking generic." />
        ))}
      </div>
      <CenteredCta theme={theme} />
    </section>
  );
}

function SplitHeroLayout({
  store,
  theme,
}: {
  store: PublicStore;
  theme: StorefrontTheme;
}) {
  return (
    <section className="grid gap-6 lg:grid-cols-2 lg:items-stretch">
      <StoryPanel
        theme={theme}
        eyebrow="Store launch"
        title={store.name}
        body="A practical split layout that gives equal weight to the store message and launch actions."
        large
      />
      <Card
        theme={theme}
        title="Open with confidence"
        body="Highlight collections, mention delivery expectations, or use this area for a featured launch offer."
      >
        <div className="grid gap-3 sm:grid-cols-2">
          <MiniStat theme={theme} label="Visual style" value="Structured" />
          <MiniStat theme={theme} label="Best for" value="Focused launches" />
        </div>
      </Card>
    </section>
  );
}

function CatalogFirstLayout({
  store,
  theme,
}: {
  store: PublicStore;
  theme: StorefrontTheme;
}) {
  return (
    <section className="space-y-8">
      <HeroTitle store={store} theme={theme} />
      <div className="grid gap-4 md:grid-cols-4">
        {["Featured", "New in", "Best sellers", "Collections"].map((item) => (
          <Card key={item} theme={theme} title={item} body="Use this layout when browsing should start immediately." compact />
        ))}
      </div>
    </section>
  );
}

function StoryLedLayout({
  store,
  theme,
}: {
  store: PublicStore;
  theme: StorefrontTheme;
}) {
  return (
    <section className="grid gap-10 lg:grid-cols-[minmax(0,0.8fr)_minmax(0,1.2fr)] lg:items-center">
      <div className="space-y-4">
        <p
          className="text-[11px] font-semibold uppercase tracking-[0.24em]"
          style={{ color: `${theme.colors.accent}D9` }}
        >
          Story-led layout
        </p>
        <h1 className="text-5xl font-medium tracking-tight sm:text-6xl" style={headingStyle(theme)}>
          {store.name}
        </h1>
      </div>
      <StoryPanel
        theme={theme}
        eyebrow="Brand perspective"
        title="Tell customers why this store exists before you ask them to browse."
        body="This layout works well for founder-led brands, crafted products, and stores where the point of view matters as much as the catalog."
      />
    </section>
  );
}

function MinimalLayout({
  store,
  theme,
}: {
  store: PublicStore;
  theme: StorefrontTheme;
}) {
  return (
    <section className="mx-auto max-w-3xl space-y-6 py-10 text-center">
      <HeroTitle store={store} theme={theme} align="center" />
      <p className="text-lg leading-8 opacity-75">
        A restrained storefront for merchants who want quiet confidence and
        plenty of breathing room.
      </p>
      <CenteredCta theme={theme} />
    </section>
  );
}

function BoldPromoLayout({
  store,
  theme,
}: {
  store: PublicStore;
  theme: StorefrontTheme;
}) {
  return (
    <section
      className="rounded-[2rem] border p-8 sm:p-10"
      style={{
        background: `linear-gradient(135deg, ${theme.colors.primary}, ${theme.colors.accent})`,
        color: "#fff",
        borderColor: `${theme.colors.primary}66`,
      }}
    >
      <p className="text-[11px] font-semibold uppercase tracking-[0.24em] text-white/75">
        Promotional layout
      </p>
      <h1 className="mt-4 text-5xl font-medium tracking-tight sm:text-6xl" style={headingStyle(theme)}>
        {store.name}
      </h1>
      <p className="mt-4 max-w-2xl text-lg leading-8 text-white/85">
        For launches, seasonal drops, and stores that want the first fold to
        carry stronger energy.
      </p>
      <div className="mt-8 flex flex-wrap gap-3">
        <PromoButton>Browse launch drop</PromoButton>
        <PromoGhostButton>Read the story</PromoGhostButton>
      </div>
    </section>
  );
}

function CompactLayout({
  store,
  theme,
}: {
  store: PublicStore;
  theme: StorefrontTheme;
}) {
  return (
    <section className="space-y-5">
      <HeroTitle store={store} theme={theme} />
      <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_18rem]">
        <div className="grid gap-3 sm:grid-cols-2">
          {["Featured arrivals", "Fast-moving picks", "Everyday essentials", "Seasonal updates"].map((item) => (
            <Card key={item} theme={theme} title={item} body="A tighter storefront rhythm for practical, category-first shops." compact />
          ))}
        </div>
        <StoryPanel
          theme={theme}
          eyebrow="Quick scan"
          title="Dense enough to browse quickly without losing polish."
          body="Ideal for stores with wider catalogs or a more operational, no-fuss brand voice."
        />
      </div>
    </section>
  );
}

function HeroTitle({
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
        style={headingStyle(theme)}
      >
        {store.name}
      </h1>
      <p className="mt-4 text-base leading-7 opacity-75">
        Theme preset: {theme.preset.replace("-", " ")} · {theme.layout.replace("-", " ")}
      </p>
    </div>
  );
}

function ActionRow({ theme }: { theme: StorefrontTheme }) {
  return (
    <div className="flex flex-wrap gap-3">
      <PrimaryButton theme={theme}>Browse the store</PrimaryButton>
      <SecondaryButton theme={theme}>See featured collections</SecondaryButton>
    </div>
  );
}

function CenteredCta({ theme }: { theme: StorefrontTheme }) {
  return (
    <div className="flex justify-center">
      <PrimaryButton theme={theme}>Start browsing</PrimaryButton>
    </div>
  );
}

function Card({
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
  children?: React.ReactNode;
}) {
  return (
    <section
      className={compact ? "space-y-3 p-5" : "space-y-4 p-6"}
      style={surfaceStyle(theme)}
    >
      <h2 className="text-lg font-semibold" style={headingStyle(theme)}>
        {title}
      </h2>
      <p className="text-sm leading-6 opacity-75">{body}</p>
      {children}
    </section>
  );
}

function StoryPanel({
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
      <h2 className={large ? "text-4xl font-medium tracking-tight" : "text-2xl font-medium tracking-tight"} style={headingStyle(theme)}>
        {title}
      </h2>
      <p className="text-sm leading-7 opacity-75">{body}</p>
    </section>
  );
}

function MetaChip({
  label,
  value,
  theme,
}: {
  label: string;
  value: string;
  theme: StorefrontTheme;
}) {
  return (
    <span
      className="inline-flex items-center gap-2 rounded-full border px-3 py-1.5 text-sm shadow-sm"
      style={{
        background: `${theme.colors.surface}E6`,
        borderColor: `${theme.colors.primary}22`,
      }}
    >
      <span className="text-[11px] font-semibold uppercase tracking-[0.14em] opacity-55">
        {label}
      </span>
      <span className="font-medium">{value}</span>
    </span>
  );
}

function MiniStat({
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

function PrimaryButton({
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

function SecondaryButton({
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

function PromoButton({ children }: { children: ReactNode }) {
  return (
    <button
      type="button"
      className="rounded-full bg-white px-5 py-3 text-sm font-semibold text-neutral-900"
    >
      {children}
    </button>
  );
}

function PromoGhostButton({ children }: { children: ReactNode }) {
  return (
    <button
      type="button"
      className="rounded-full border border-white/35 px-5 py-3 text-sm font-semibold text-white"
    >
      {children}
    </button>
  );
}

function StoreNotFound({ slug }: { slug: string }) {
  return (
    <main className="mx-auto flex min-h-screen max-w-2xl flex-col items-start justify-center gap-6 px-6 py-20">
      <p className="text-xs font-semibold uppercase tracking-[0.24em] text-neutral-500">
        Store not found
      </p>
      <h1 className="text-5xl font-medium tracking-tight text-neutral-900">
        Nothing here yet
      </h1>
      <p className="max-w-xl text-lg leading-8 text-neutral-600">
        {slug
          ? `We couldn't find a store at "${slug}". The URL may be wrong, or the store isn't live yet.`
          : "This domain isn't pointed at a live store yet."}
      </p>
    </main>
  );
}

function themeStyle(theme: StorefrontTheme): CSSProperties {
  return {
    ["--store-heading-font" as string]: fontStacks[theme.typography.headingFont],
    ["--store-body-font" as string]: fontStacks[theme.typography.bodyFont],
    ["--store-radius" as string]: themeRadius(theme),
    ["--store-spacing" as string]: themeSpacing(theme),
  };
}

function headingStyle(theme: StorefrontTheme): CSSProperties {
  return { fontFamily: "var(--store-heading-font)" };
}

function surfaceStyle(theme: StorefrontTheme): CSSProperties {
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
