import type { Metadata } from "next";

import { SeoLanding } from "@/components/marketing/SeoLanding";

export const metadata: Metadata = {
  title: "Etsy Alternative — Own Your Own Website",
  description:
    "An Etsy alternative where you own your own website: your domain, your brand, your customer list, and no per-sale platform fees. Editorial storefront, 90 days free.",
  alternates: { canonical: "/etsy-alternative" },
  openGraph: {
    title: "An Etsy alternative you actually own",
    description:
      "Your own domain, your own brand, your own customer list — and no per-sale platform fees. 90 days free.",
    url: "https://mark8ly.com/etsy-alternative",
  },
};

export default function EtsyAlternativePage() {
  return (
    <SeoLanding
      eyebrow="Etsy alternative"
      title={
        <>
          An Etsy alternative
          <br />
          you actually own.
        </>
      }
      lede="Mark8ly is an Etsy alternative where the shop is yours: your own domain, your own brand, your own customer list, and no per-sale platform fees. Ninety days free to try it."
      intro={[
        "Etsy is a marketplace, and a marketplace is someone else's shopping mall. You rent a stall, you compete with near-identical listings a scroll away, and you pay for the privilege on every sale — listing fees, transaction fees, payment processing, and more if you want to be seen. Worst of all, the customer belongs to Etsy, not to you.",
        "Mark8ly is the opposite of that. It's your own website, on your own domain, with your own brand at the top and your own customer list underneath. When someone buys, they're buying from you — and you keep their relationship, their email, and the next sale.",
        "There's no per-sale platform fee. You pay only your payment processor's standard rate; we don't add a cut on top. Ninety days free to start, then plans from $29 a month — a flat, predictable cost instead of a stack of fees that grows with every order.",
      ]}
      competitorName="Etsy"
      comparisonNote="Etsy is a marketplace with listing, transaction, and payment fees per sale; Mark8ly is your own website with no platform cut."
      comparison={[
        { label: "Whose shop is it", mark8ly: "Yours", them: "Etsy's marketplace" },
        {
          label: "Your own domain & brand",
          mark8ly: "Included",
          them: "Etsy storefront under etsy.com",
        },
        {
          label: "Platform fee per sale",
          mark8ly: "None",
          them: "Listing + transaction + payment fees",
        },
        {
          label: "Who owns the customer",
          mark8ly: "You do",
          them: "Etsy",
        },
        {
          label: "Competing listings alongside yours",
          mark8ly: "None — it's your site",
          them: "The whole marketplace",
        },
        {
          label: "Take your data when you leave",
          mark8ly: "One click, any time",
          them: "Limited export",
        },
      ]}
      sections={[
        {
          heading: "Your brand, not a stall in a mall",
          body: [
            "On Etsy, your shop lives under etsy.com and looks like every other shop under etsy.com. On Mark8ly, it lives on your own domain and looks like you. The default storefront is editorial — designed, restrained, product-first — so your work stands on its own instead of blending into an endless grid.",
            "That difference compounds over time. A distinct shop on your own domain builds a brand customers remember and return to. A listing in a marketplace builds Etsy's brand.",
          ],
        },
        {
          heading: "Keep the customer, keep the margin",
          body: [
            "The most valuable thing a marketplace keeps from you is the customer. On Mark8ly, your customer list is yours — you can reach them again, tell them about a new drop, and build repeat business without paying to be re-seen.",
            "And the economics are simpler. No listing fees, no per-sale platform cut — just your processor's standard rate and a flat monthly plan. What you earn on a sale is what you keep.",
          ],
        },
        {
          heading: "Run it alongside Etsy, or instead of it",
          body: [
            "You don't have to leave Etsy on day one. Many sellers open a Mark8ly shop as the home base they own, and point their best customers there over time. If you can write an email, you can have it live in an afternoon, with real humans to help.",
            "And it stays yours: one-click export of products, customers, and orders means you're never locked in. The shop is a place you own, not a lease you're stuck with.",
          ],
        },
      ]}
      faq={[
        {
          question: "How is Mark8ly different from Etsy?",
          answer:
            "Etsy is a marketplace where you rent a listing and pay fees per sale, and the customer belongs to Etsy. Mark8ly is your own website — your domain, your brand, your customer list — with no per-sale platform fee.",
        },
        {
          question: "Are there listing or transaction fees?",
          answer:
            "No. There are no listing fees and no platform transaction fee. You pay only your payment processor's standard rate, plus a flat monthly plan starting at $29 after 90 days free.",
        },
        {
          question: "Can I keep selling on Etsy too?",
          answer:
            "Yes. Many sellers run a Mark8ly store as the shop they own alongside their Etsy listings, and shift repeat customers over time.",
        },
        {
          question: "Do I own my customer list?",
          answer:
            "Yes. Your customers are yours — you can reach them for repeat sales, and you can export your products, customers, and orders in one click at any time.",
        },
      ]}
      ctaHeading={
        <>
          Own your shop,
          <br />
          not a stall.
        </>
      }
      ctaBody="Your own domain, your own brand, your own customers — and no per-sale platform fee. Ninety days free, no card required."
    />
  );
}
