import Link from "next/link";

interface FooterColumnProps {
  title: string;
  links: Array<{ href: string; label: string }>;
}

/**
 * Marketing footer.
 *
 * Editorial layout: brand block left, three link columns right.
 * Solid ink background — the only place on the site where the
 * deep ink color dominates, used as a visual full-stop after
 * every page. Pure server component.
 */
export function Footer() {
  const currentYear = new Date().getFullYear();

  return (
    <footer className="bg-ink-900 text-paper-300">
      <div className="mx-auto max-w-6xl px-6 py-20">
        <div className="grid gap-12 md:grid-cols-[1.4fr_3fr]">
          <div>
            <Link
              href="/"
              aria-label="mark8ly — home"
              className="inline-flex items-center text-paper-50"
            >
              <span className="font-serif text-3xl font-medium tracking-[-0.025em]">
                mark8ly
              </span>
            </Link>
            <p className="mt-5 max-w-sm text-[0.9375rem] leading-relaxed text-paper-400">
              Quiet commerce for people who make things. Launch a
              storefront in an afternoon, then keep your margins for
              yourself.
            </p>
          </div>

          <div className="grid grid-cols-2 gap-10 sm:grid-cols-3 lg:grid-cols-5">
            <FooterColumn
              title="Product"
              links={[
                { href: "/#features", label: "Features" },
                { href: "/#pricing", label: "Pricing" },
                { href: "/integrations", label: "Integrations" },
              ]}
            />
            <FooterColumn
              title="Compare"
              links={[
                { href: "/shopify-alternative", label: "Shopify alternative" },
                { href: "/etsy-alternative", label: "Etsy alternative" },
                { href: "/ecommerce-for-makers", label: "For makers" },
                { href: "/sell-online-india", label: "Sell in India" },
              ]}
            />
            <FooterColumn
              title="Resources"
              links={[
                { href: "/help", label: "Help" },
                { href: "/guides", label: "Guides" },
                { href: "/blog", label: "Journal" },
                { href: "/#faq", label: "Questions" },
              ]}
            />
            <FooterColumn
              title="Company"
              links={[
                { href: "/about", label: "About" },
                { href: "/contact", label: "Contact" },
              ]}
            />
            <FooterColumn
              title="Legal"
              links={[
                { href: "/privacy", label: "Privacy" },
                { href: "/terms", label: "Terms" },
                { href: "/delete-account", label: "Delete account" },
                { href: "/cookies", label: "Cookies" },
                { href: "/acceptable-use", label: "Acceptable use" },
                { href: "/refunds", label: "Refunds" },
                { href: "/security", label: "Security" },
                { href: "/legal", label: "All policies" },
              ]}
            />
          </div>
        </div>

        <div className="mt-16 border-t border-paper-600/20 pt-8">
          <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
            <p className="text-sm text-paper-500">
              © {currentYear} mark8ly · A Tesserix product
            </p>
            <a
              href="mailto:hello@mark8ly.com"
              className="inline-flex h-11 items-center text-sm text-paper-400 hover:text-paper-50"
            >
              hello@mark8ly.com
            </a>
          </div>
          {/* Operator disclosure. Named here so a merchant can match the entity
              on their invoice and settlement to the product they signed up to. */}
          <p className="mt-6 text-xs leading-relaxed text-paper-500">
            Powered by Zivana Innovations LLP, part of Tesserix Pty Ltd
            (ACN 694 070 865 · ABN 59 694 070 865)
          </p>
        </div>
      </div>
    </footer>
  );
}

function FooterColumn({ title, links }: FooterColumnProps) {
  return (
    <div>
      <h2 className="mb-5 text-xs font-medium uppercase tracking-[0.16em] text-paper-50">
        {title}
      </h2>
      <ul className="space-y-3">
        {links.map((l) => (
          <li key={`${l.href}-${l.label}`}>
            <Link
              href={l.href}
              className="text-[0.9375rem] text-paper-400 hover:text-paper-50"
            >
              {l.label}
            </Link>
          </li>
        ))}
      </ul>
    </div>
  );
}
