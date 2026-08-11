import Link from "next/link";

import { MarketingPage, PageHero } from "@/components/marketing/primitives";

export const metadata = {
  title: "Contact",
  description:
    "Email us directly. We&rsquo;re a small team and we actually read what lands in the inbox. No ticket queues, no chatbots, no form maze.",
  alternates: { canonical: "/contact" },
  openGraph: {
    title: "Contact · Mark8ly",
    description:
      "Say hello. hello@mark8ly.com for support, partners@mark8ly.com for partnerships, legal@mark8ly.com for press.",
    url: "/contact",
  },
};

export default function ContactPage() {
  return (
    <MarketingPage>
      <PageHero
        eyebrow="Contact"
        title={<>Say hello.</>}
        lede="We&rsquo;re a small team and we actually read what lands in the inbox. No ticket queues, no chatbots, no form maze."
      />

      <section className="border-t border-border-subtle py-20 sm:py-28">
        <div className="mx-auto grid max-w-6xl gap-12 px-6 lg:grid-cols-[1fr_2fr] lg:gap-16">
          <div>
            <p className="eyebrow mb-5">By email</p>
          </div>
          <dl className="space-y-10 lg:max-w-xl">
            {channels.map((channel) => (
              <div
                key={channel.email}
                className="border-t border-border-subtle pt-8"
              >
                <dt className="font-serif text-2xl text-foreground">
                  {channel.title}
                </dt>
                <dd className="mt-3 text-foreground-secondary leading-relaxed">
                  {channel.description}
                </dd>
                <dd className="mt-4">
                  <a
                    href={`mailto:${channel.email}`}
                    className="text-foreground underline decoration-moss-700 decoration-2 underline-offset-4 hover:text-moss-700"
                  >
                    {channel.email}
                  </a>
                </dd>
              </div>
            ))}
          </dl>
        </div>
      </section>

      <section className="border-t border-border-subtle py-20 sm:py-28">
        <div className="mx-auto grid max-w-6xl gap-12 px-6 lg:grid-cols-[1fr_2fr] lg:gap-16">
          <div>
            <p className="eyebrow mb-5">Where we are</p>
          </div>
          <div className="space-y-6 text-foreground-secondary leading-relaxed lg:max-w-xl">
            <p>
              Australia. We work remote-first across a small distributed
              team, which means there is almost always someone awake when you
              write to us.
            </p>
            <p>
              If you&rsquo;re local and want to meet, email{" "}
              <a
                href="mailto:hello@mark8ly.com"
                className="text-foreground underline decoration-moss-700 decoration-2 underline-offset-4 hover:text-moss-700"
              >
                hello@mark8ly.com
              </a>
              .
            </p>
          </div>
        </div>
      </section>

      <section className="border-t border-border-subtle py-20 sm:py-24">
        <div className="mx-auto max-w-6xl px-6">
          <div className="flex flex-wrap items-center gap-x-8 gap-y-4">
            <Link
              href="/onboarding"
              className="inline-flex h-12 items-center rounded-md bg-primary px-6 text-base font-medium text-primary-foreground hover:bg-primary-hover"
            >
              Open your store
            </Link>
            <Link href="/about" className="btn-ghost">
              Learn who we are
            </Link>
          </div>
        </div>
      </section>
    </MarketingPage>
  );
}

const channels = [
  {
    title: "General support",
    description:
      "Questions about setup, billing, or anything that isn&rsquo;t working the way you expected. The inbox we check first.",
    email: "hello@mark8ly.com",
  },
  {
    title: "Partnerships",
    description:
      "Payment processors, shipping carriers, marketing tools, agencies, or anyone who wants to build with us.",
    email: "partners@mark8ly.com",
  },
  {
    title: "Legal & press",
    description:
      "Privacy requests, compliance questions, press enquiries, or anything that needs a considered reply.",
    email: "legal@mark8ly.com",
  },
];
