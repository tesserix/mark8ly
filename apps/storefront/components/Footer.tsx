import Link from "next/link";

import type {
  FooterLinkItem,
  FooterSection,
  StorefrontBranding,
} from "@/lib/api/marketplace-api";
import { safeUrl } from "@/lib/safeUrl";

interface FooterProps {
  branding?: StorefrontBranding | null;
}

function defaultCopyright(): string {
  return `© ${new Date().getFullYear()} — All rights reserved.`;
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
  if (!href) {
    return (
      <span className="text-sm text-foreground-secondary">{item.label}</span>
    );
  }
  const external = item.kind === "url" && isExternal(href);
  if (external) {
    return (
      <a
        href={href}
        target="_blank"
        rel="noreferrer"
        className="text-sm text-foreground-secondary transition-colors hover:text-moss-700"
      >
        {item.label}
      </a>
    );
  }
  return (
    <Link
      href={href}
      className="text-sm text-foreground-secondary transition-colors hover:text-moss-700"
    >
      {item.label}
    </Link>
  );
}

function SocialRow({ branding }: { branding: StorefrontBranding }) {
  const links: Array<{ key: string; label: string; url?: string | null }> = [
    { key: "instagram", label: "Instagram", url: branding.social_instagram },
    { key: "twitter", label: "X", url: branding.social_twitter },
    { key: "facebook", label: "Facebook", url: branding.social_facebook },
    { key: "tiktok", label: "TikTok", url: branding.social_tiktok },
    { key: "youtube", label: "YouTube", url: branding.social_youtube },
  ].filter((l) => !!l.url);

  if (links.length === 0) return null;

  return (
    <ul className="mt-6 flex flex-wrap gap-4 text-sm">
      {links.map((l) => (
        <li key={l.key}>
          <a
            href={safeUrl(l.url)}
            target="_blank"
            rel="noreferrer"
            className="text-foreground-secondary transition-colors hover:text-moss-700"
          >
            {l.label}
          </a>
        </li>
      ))}
    </ul>
  );
}

function Section({ section }: { section: FooterSection }) {
  return (
    <div>
      <p className="font-serif text-base font-medium text-foreground">
        {section.label}
      </p>
      <ul className="mt-4 space-y-2">
        {section.items.map((item, i) => (
          <li key={`${item.label}-${i}`}>
            <FooterLink item={item} />
          </li>
        ))}
      </ul>
    </div>
  );
}

export function Footer({ branding }: FooterProps) {
  const tagline = branding?.footer_tagline?.trim() || null;
  const copyright = branding?.footer_copyright?.trim() || defaultCopyright();
  const sections = branding?.footer_sections ?? [];

  return (
    <footer className="mt-24 border-t border-border-subtle bg-background">
      <div className="mx-auto max-w-6xl px-6 py-16 sm:py-20">
        <div className="grid gap-12 md:grid-cols-3 md:gap-10">
          {/* Tagline + social — left */}
          <div className="md:col-span-1">
            {tagline ? (
              <p className="font-serif text-lg text-foreground">{tagline}</p>
            ) : null}
            {branding ? <SocialRow branding={branding} /> : null}
          </div>

          {/* Sections — spans 2 columns on md+ */}
          <div className="md:col-span-2">
            {sections.length > 0 ? (
              <div className="grid gap-10 sm:grid-cols-2 lg:grid-cols-3">
                {sections.slice(0, 6).map((section, i) => (
                  <Section key={`${section.label}-${i}`} section={section} />
                ))}
              </div>
            ) : null}
          </div>
        </div>

        <div className="mt-16 flex flex-col items-start justify-between gap-4 border-t border-border-subtle pt-6 text-xs text-foreground-secondary sm:flex-row sm:items-center">
          <p>{copyright}</p>
        </div>
      </div>
    </footer>
  );
}
