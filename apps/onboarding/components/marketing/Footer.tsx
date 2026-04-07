"use client";

import Link from "next/link";

/**
 * Marketing footer — ported from mark8ly_backup with the full link set:
 * Product / Resources / Company columns plus the brand block. Items that
 * don't have a real destination yet point at the StubPage placeholders.
 */
export function Footer() {
  const currentYear = new Date().getFullYear();

  return (
    <footer className="bg-foreground py-16 px-6">
      <div className="max-w-6xl mx-auto">
        <div className="grid grid-cols-1 md:grid-cols-4 gap-10 mb-12">
          {/* Brand */}
          <div className="md:col-span-1">
            <Link
              href="/"
              className="flex items-center mb-4 hover:opacity-80 transition-opacity"
            >
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src="/icon-192.png"
                alt="mark8ly icon"
                className="h-8 w-auto brightness-0 invert"
              />
              <span className="text-lg font-serif font-medium tracking-[-0.015em] text-warm-100">
                mark8ly
              </span>
            </Link>
            <p className="text-warm-400 text-sm leading-relaxed">
              The simplest way to launch and grow your online store. Built for
              creators, makers, and small businesses.
            </p>
          </div>

          <FooterColumn
            title="Product"
            links={[
              { href: "/#features", label: "Features" },
              { href: "/#pricing", label: "Pricing" },
              { href: "/presentation", label: "Demo" },
              { href: "/integrations", label: "Integrations" },
            ]}
          />

          <FooterColumn
            title="Resources"
            links={[
              { href: "/help", label: "Help Center" },
              { href: "/guides", label: "Guides" },
              { href: "/blog", label: "Blog" },
              { href: "/#faq", label: "FAQ" },
            ]}
          />

          <FooterColumn
            title="Company"
            links={[
              { href: "/about", label: "About" },
              { href: "/contact", label: "Contact" },
              { href: "/privacy", label: "Privacy Policy" },
              { href: "/terms", label: "Terms of Service" },
              { href: "/legal", label: "Security & Compliance" },
            ]}
          />
        </div>

        <div className="pt-8 border-t border-warm-800/40 flex flex-col sm:flex-row justify-between items-center gap-4">
          <p className="text-sm text-warm-500">
            {`© ${currentYear} mark8ly by Tesserix · Crafted with care for people who make things.`}
          </p>
          <div className="flex items-center gap-4">
            <a
              href="mailto:support@mark8ly.com"
              className="text-warm-500 hover:text-warm-300 transition-colors"
              aria-label="Email us"
            >
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M4 4h16c1.1 0 2 .9 2 2v12c0 1.1-.9 2-2 2H4c-1.1 0-2-.9-2-2V6c0-1.1.9-2 2-2z" />
                <polyline points="22,6 12,13 2,6" />
              </svg>
            </a>
            <Link
              href="/contact"
              className="text-warm-500 hover:text-warm-300 transition-colors"
              aria-label="Contact"
            >
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
              </svg>
            </Link>
          </div>
        </div>
      </div>
    </footer>
  );
}

interface FooterColumnProps {
  title: string;
  links: Array<{ href: string; label: string }>;
}
function FooterColumn({ title, links }: FooterColumnProps) {
  return (
    <div>
      <h4 className="text-warm-100 font-medium mb-4">{title}</h4>
      <ul className="space-y-3 text-sm text-warm-400">
        {links.map((l) => (
          <li key={`${l.href}-${l.label}`}>
            <Link href={l.href} className="hover:text-warm-100 transition-colors">
              {l.label}
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
