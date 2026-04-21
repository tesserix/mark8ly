import Link from "next/link";
import type { ComponentType, SVGProps } from "react";

import type {
  FooterLinkItem,
  FooterSection,
  StorefrontBranding,
} from "@/lib/api/marketplace-api";
import { safeUrl } from "@/lib/safeUrl";
import {
  FacebookIcon,
  InstagramIcon,
  TikTokIcon,
  XIcon,
  YouTubeIcon,
} from "./SocialIcons";

interface FooterProps {
  branding?: StorefrontBranding | null;
  storeName?: string | null;
}

function defaultCopyright(storeName: string | null): string {
  const year = new Date().getFullYear();
  return storeName
    ? `© ${year} ${storeName} — All rights reserved.`
    : `© ${year} — All rights reserved.`;
}

function isExternal(url: string): boolean {
  return url.startsWith("http://") || url.startsWith("https://");
}

function itemHref(item: FooterLinkItem): string | null {
  if (item.kind === "page") {
    return item.page_slug ? `/pages/${item.page_slug}` : null;
  }
  const u = item.url ?? null;
  if (u === null) return null;
  const safe = safeUrl(u);
  return safe === "#" ? null : safe;
}

function FooterLink({ item }: { item: FooterLinkItem }) {
  const href = itemHref(item);
  const linkClass =
    "text-sm text-[color:var(--storefront-text,var(--ink-900))]/70 transition-colors hover:text-[color:var(--storefront-accent,var(--moss-700))]";
  if (!href) {
    return (
      <span className="text-sm text-[color:var(--storefront-text,var(--ink-900))]/60">
        {item.label}
      </span>
    );
  }
  if (item.kind === "url" && isExternal(href)) {
    return (
      <a href={href} target="_blank" rel="noreferrer" className={linkClass}>
        {item.label}
      </a>
    );
  }
  return (
    <Link href={href} className={linkClass}>
      {item.label}
    </Link>
  );
}

type SocialLink = {
  key: string;
  label: string;
  url?: string | null;
  Icon: ComponentType<SVGProps<SVGSVGElement>>;
};

function SocialRow({ branding }: { branding: StorefrontBranding }) {
  const links: SocialLink[] = [
    { key: "instagram", label: "Instagram", url: branding.social_instagram, Icon: InstagramIcon },
    { key: "twitter", label: "X", url: branding.social_twitter, Icon: XIcon },
    { key: "facebook", label: "Facebook", url: branding.social_facebook, Icon: FacebookIcon },
    { key: "tiktok", label: "TikTok", url: branding.social_tiktok, Icon: TikTokIcon },
    { key: "youtube", label: "YouTube", url: branding.social_youtube, Icon: YouTubeIcon },
  ];
  const present = links.filter((l) => !!l.url);
  if (present.length === 0) return null;

  return (
    <ul className="flex flex-wrap items-center gap-5">
      {present.map(({ key, label, url, Icon }) => (
        <li key={key}>
          <a
            href={safeUrl(url)}
            target="_blank"
            rel="noreferrer"
            aria-label={label}
            title={label}
            className="inline-flex text-[color:var(--storefront-text,var(--ink-900))]/60 transition-colors hover:text-[color:var(--storefront-accent,var(--moss-700))]"
          >
            <Icon className="h-5 w-5" />
            <span className="sr-only">{label}</span>
          </a>
        </li>
      ))}
    </ul>
  );
}

function Section({ section }: { section: FooterSection }) {
  if (!section.items || section.items.length === 0) return null;
  return (
    <div>
      <p className="text-[10px] font-semibold uppercase tracking-[0.2em] text-[color:var(--storefront-text,var(--ink-900))]">
        {section.label}
      </p>
      <ul className="mt-4 space-y-2.5">
        {section.items.map((item, i) => (
          <li key={`item-${i}`}>
            <FooterLink item={item} />
          </li>
        ))}
      </ul>
    </div>
  );
}

function BrandHero({
  storeName,
  logoUrl,
}: {
  storeName: string | null;
  logoUrl: string | null;
}) {
  if (logoUrl) {
    return (
      // Logo display uses a plain <img> intentionally — merchants upload
      // arbitrary remote URLs, and Next's <Image> would require whitelisting
      // every tenant domain. We still set explicit intrinsic dimensions +
      // lazy-load attributes so the browser can reserve space (no CLS) and
      // skip the fetch on mount when the footer is below the fold.
      // eslint-disable-next-line @next/next/no-img-element
      <img
        src={logoUrl}
        alt={storeName ?? "Store logo"}
        width={240}
        height={48}
        loading="lazy"
        decoding="async"
        className="h-12 w-auto max-w-[240px] object-contain object-left"
      />
    );
  }
  if (storeName) {
    return (
      <p
        className="font-[family-name:var(--storefront-heading-font,var(--font-serif))] text-[clamp(2.25rem,5vw,3.5rem)] font-normal leading-[1.05] tracking-[-0.015em] text-[color:var(--storefront-text,var(--ink-900))]"
      >
        {storeName}
      </p>
    );
  }
  return null;
}

export function Footer({ branding, storeName }: FooterProps) {
  const tagline = branding?.footer_tagline?.trim() || null;
  const name = storeName?.trim() || null;
  const copyright = branding?.footer_copyright?.trim() || defaultCopyright(name);
  const logoUrl = branding?.logo_url?.trim() || null;
  const sections = (branding?.footer_sections ?? []).filter(
    (s) => s.label && s.label.trim().length > 0,
  );
  const showPoweredBy = branding?.show_powered_by !== false;
  const hasHero = Boolean(name || logoUrl);
  const hasSocials = Boolean(
    branding?.social_instagram ||
      branding?.social_twitter ||
      branding?.social_facebook ||
      branding?.social_tiktok ||
      branding?.social_youtube,
  );

  return (
    <footer
      className="mt-24 border-t border-[color:var(--storefront-text,var(--ink-900))]/10 bg-[color:var(--storefront-background,var(--paper-200))]"
      role="contentinfo"
    >
      <div className="mx-auto max-w-6xl px-6 sm:px-8">
        {/* ── Brand row ───────────────────────────────────────── */}
        {(hasHero || tagline) && (
          <div className="pt-14 pb-10 sm:pt-16">
            <BrandHero storeName={name} logoUrl={logoUrl} />
            {tagline && (
              <p
                className={`${hasHero ? "mt-3" : ""} max-w-[60ch] text-[15px] leading-relaxed text-[color:var(--storefront-text,var(--ink-900))]/65`}
              >
                {tagline}
              </p>
            )}
          </div>
        )}

        {/* ── Middle row: socials + sections ─────────────────── */}
        <div className="border-t border-[color:var(--storefront-text,var(--ink-900))]/10 pt-10 pb-12 sm:pt-12">
          <div className="grid gap-10 md:grid-cols-12 md:gap-12">
            {/* Socials — 3/12 on md+, full-width row above sections on mobile */}
            {hasSocials && branding ? (
              <div className="md:col-span-3">
                <SocialRow branding={branding} />
              </div>
            ) : null}

            {/* Link sections — spans remaining columns */}
            {sections.length > 0 && (
              <div className={hasSocials ? "md:col-span-9" : "md:col-span-12"}>
                <div className="grid gap-x-10 gap-y-10 sm:grid-cols-2 md:grid-cols-3">
                  {sections.slice(0, 6).map((section, i) => (
                    <Section key={`section-${i}`} section={section} />
                  ))}
                </div>
              </div>
            )}
          </div>
        </div>

        {/* ── Bottom bar ─────────────────────────────────────── */}
        <div className="flex flex-wrap items-center justify-between gap-3 border-t border-[color:var(--storefront-text,var(--ink-900))]/10 py-6 text-[11px] text-[color:var(--storefront-text,var(--ink-900))]/55">
          <p>{copyright}</p>
          {showPoweredBy && (
            <p>
              Powered by{" "}
              <a
                href="https://mark8ly.com"
                target="_blank"
                rel="noreferrer"
                className="font-medium text-[color:var(--storefront-accent,var(--moss-700))] transition-colors hover:underline underline-offset-4"
              >
                mark8ly
              </a>
            </p>
          )}
        </div>
      </div>
    </footer>
  );
}
