import type { Metadata } from "next";

import { SeoLanding } from "@/components/marketing/SeoLanding";

export const metadata: Metadata = {
  title: "Shopify Alternative With No Transaction Fees",
  description:
    "Looking for a Shopify alternative with no platform transaction fees? Mark8ly gives you an editorial storefront, your own domain, and 90 days free. Keep your margins.",
  alternates: { canonical: "/shopify-alternative" },
  openGraph: {
    title: "A Shopify alternative that doesn't skim your sales",
    description:
      "No platform transaction fees. A designer-led storefront out of the box. 90 days free, no card required.",
    url: "https://mark8ly.com/shopify-alternative",
  },
};

export default function ShopifyAlternativePage() {
  return (
    <SeoLanding
      eyebrow="Shopify alternative"
      title={
        <>
          A Shopify alternative
          <br />
          that keeps your margins.
        </>
      }
      lede="Mark8ly is a quiet, considered commerce platform with no platform transaction fees, a storefront that looks designed rather than templated, and ninety days to try it before you pay anything."
      intro={[
        "If you're searching for a Shopify alternative, it's usually for one of two reasons: the fees, or the sameness. Shopify Basic starts around $29 a month and, unless you route every sale through Shopify Payments, adds a 2% platform fee on top of your processor's cut. On thin margins — which is most independent commerce — that 2% is the difference between a good month and a flat one.",
        "Mark8ly takes nothing from your sales. Your payment processor charges their standard rate (roughly 2% for UPI, 2–3% for cards) and that's the entire cost of taking money. We don't add a platform fee, ever. You keep what you earn, which is the whole point of running your own shop.",
        "The second reason is how Shopify stores look. Most run a handful of the same themes, so a thousand shops share one silhouette. Mark8ly ships one editorial storefront designed by people who've actually sold things — quiet typography, generous whitespace, real attention to product pages — so your shop reads as yours from the first visit.",
      ]}
      competitorName="Shopify"
      comparisonNote="Competitor figures are Shopify's own published Basic-plan pricing and policies, verified against their regional pricing pages."
      comparison={[
        { label: "Free to try", mark8ly: "90 days", them: "3 days" },
        {
          label: "Platform fee per sale",
          mark8ly: "None",
          them: "2%, unless you use Shopify Payments",
        },
        {
          label: "Default storefront design",
          mark8ly: "Editorial, designer-led",
          them: "Generic templates",
        },
        {
          label: "Use your own domain",
          mark8ly: "Included",
          them: "Bring your own",
        },
        {
          label: "Local payments (UPI, wallets)",
          mark8ly: "Built in",
          them: "Limited by region",
        },
        {
          label: "Take your data when you leave",
          mark8ly: "One click, any time",
          them: "CSV export",
        },
      ]}
      sections={[
        {
          heading: "No platform fees, in plain terms",
          body: [
            "Every sale on Mark8ly costs you exactly what your payment processor charges — nothing more. There's no percentage skim, no per-order surcharge, no penalty for using the processor you prefer. Over a year, on real volume, that's the kind of saving that funds better packaging or a second product line.",
            "After the ninety-day free trial you choose one of three plans — Starter, Studio, or Pro — from $15 a month billed yearly. That's the whole bill. The plan is the price; the sales are yours.",
          ],
        },
        {
          heading: "A storefront that looks hired, not templated",
          body: [
            "The default Mark8ly storefront was built by designers and real merchants together. It's editorial by default — the sort of restrained, confident layout you'd normally pay a studio to build. You can make it yours without a theme licence, a design fee, or a pile of plugins.",
            "Your own domain is included, so customers land on your brand, not a subdomain. And the whole thing is set up to be legible and fast, because a shop that loads slowly is a shop that loses the sale.",
          ],
        },
        {
          heading: "Switching is low-drama",
          body: [
            "You don't need to be technical to move. If you can write an email, you can open a store on Mark8ly and have it live by the end of the afternoon. When you get stuck, real people answer real messages — no ticket carousel.",
            "And because it's your shop, you can leave whenever you like: export your products, customers, and orders in one click and take everything with you. We'd rather keep you because the product is good than because leaving is painful.",
          ],
        },
      ]}
      faq={[
        {
          question: "Does Mark8ly really charge no transaction fees?",
          answer:
            "Correct. Mark8ly adds nothing to your sales. You pay only your payment processor's standard rate — around 2% for UPI and 2–3% for cards. There is no platform fee on top, on any plan.",
        },
        {
          question: "How does the price compare to Shopify?",
          answer:
            "Plans start at $15 a month billed yearly after a 90-day free trial, with no added transaction fees. Shopify Basic starts around $29 a month with a 3-day trial and a 2% platform fee unless you use Shopify Payments.",
        },
        {
          question: "Can I use my own domain?",
          answer:
            "Yes — your own domain is included, so customers always land on your brand rather than a platform subdomain.",
        },
        {
          question: "Is it hard to migrate from Shopify?",
          answer:
            "No. You can set up a store in an afternoon, and real humans help if you get stuck. When you eventually want to leave any platform, one-click export means your data comes with you.",
        },
      ]}
      ctaHeading={
        <>
          Keep your margins.
          <br />
          Open your shop.
        </>
      }
      ctaBody="Ninety days free, no card required, and no platform fee on a single sale. Set up this afternoon."
    />
  );
}
