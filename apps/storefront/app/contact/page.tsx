import type { Metadata } from "next";
import { cookies, headers } from "next/headers";

import { decodeSessionForScope } from "@/lib/session";
import { resolveStoreSlug } from "@/lib/slug";
import { ContactForm } from "./ContactForm";

export const metadata: Metadata = {
  title: "Contact us",
  description:
    "Send a message and we'll get back to you at the email you provide.",
};

// Public contact page. Renders a form that creates a support ticket via
// the marketplace-api storefront endpoint; merchants read the resulting
// tickets on the admin dashboard. Pre-fills name/email for signed-in
// customers.
export default async function ContactPage() {
  const h = await headers();
  const storeSlug = await resolveStoreSlug(h.get("host"));
  const c = await cookies();
  const sessionCookie = c.get("mp_customer_session")?.value;
  const session = sessionCookie
    ? decodeSessionForScope(sessionCookie, { storeSlug })
    : null;

  return (
    <main className="mx-auto w-full max-w-2xl px-4 py-16 sm:px-6 sm:py-24">
      <header className="mb-10 space-y-3">
        <p className="text-[11px] font-semibold uppercase tracking-[0.18em] text-foreground-tertiary">
          Support
        </p>
        <h1 className="font-serif text-4xl tracking-[-0.02em] text-foreground sm:text-5xl">
          Get in touch
        </h1>
        <p className="max-w-prose text-sm text-foreground-secondary">
          Questions about an order, a product, or anything else? Leave a note
          and our team will be in touch — usually within one business day.
        </p>
      </header>
      <ContactForm
        storeSlug={storeSlug}
        defaultEmail={session?.email}
      />
    </main>
  );
}
