import type { Metadata } from "next";

import { SeoLanding } from "@/components/marketing/SeoLanding";

export const metadata: Metadata = {
  title: "Sell Online in India with UPI Payments",
  description:
    "Sell online in India with UPI, wallets, and cards built in, no platform fees, and your own domain. Mark8ly gives Indian sellers a designer storefront and 90 days free.",
  alternates: { canonical: "/sell-online-india" },
  openGraph: {
    title: "Sell online in India — UPI built in, no platform fees",
    description:
      "UPI, wallets, and cards behind one checkout. No platform fees. 90 days free for Indian sellers.",
    url: "https://mark8ly.com/sell-online-india",
  },
};

export default function SellOnlineIndiaPage() {
  return (
    <SeoLanding
      eyebrow="Sell online in India"
      title={
        <>
          Sell online in India,
          <br />
          UPI built in.
        </>
      }
      lede="Mark8ly lets you sell online in India with UPI, wallets, and cards behind a single checkout, no platform fees on your sales, your own domain, and ninety days to try it free."
      intro={[
        "Selling online in India comes down to one thing at the moment of truth: can the customer pay the way they already pay? For most of the country, that's UPI — a quick scan or a tap, no card numbers, no friction. A checkout that treats UPI as an afterthought loses the sale on the last screen.",
        "Mark8ly puts UPI, wallets, and cards behind one clean checkout, so however your customer prefers to pay, they can. It's built in, not bolted on — no region-locked workarounds, no forcing everyone through a single card processor that half your customers don't use.",
        "And on top of that, we take nothing from your sales. There's no platform fee \u2014 you pay only your payment gateway's rate, around 2%. Worth knowing what that 2% actually is: UPI's own merchant discount rate has been nil since 2020, so it is your gateway's fee rather than a cost of UPI itself. For Indian sellers running on real margins, keeping a platform's extra cut out of that number is decisive. Ninety days free to start, then \u20b9999 a month for Starter \u2014 or \u20b99,599 for the year, which works out at about \u20b9800 a month.",
      ]}
      competitorName="Global platforms"
      comparisonNote="Most global platforms bill in USD and treat local Indian payment methods as a regional add-on rather than the default."
      comparison={[
        { label: "Free to try", mark8ly: "90 days", them: "3–14 days" },
        {
          label: "UPI at checkout",
          mark8ly: "Built in, first-class",
          them: "Limited by region",
        },
        {
          label: "Wallets & cards",
          mark8ly: "Same single checkout",
          them: "Varies by processor",
        },
        {
          label: "Platform fee per sale",
          mark8ly: "None",
          them: "Often 2% or a per-sale cut",
        },
        {
          label: "Pricing shown in your currency",
          mark8ly: "Yes",
          them: "Often USD only",
        },
        {
          label: "Own your data",
          mark8ly: "One-click export, any time",
          them: "Partial / CSV export",
        },
      ]}
      sections={[
        {
          heading: "Payments the way India actually pays",
          body: [
            "UPI, wallets, and cards all sit behind one checkout on Mark8ly. There's no separate flow to configure, no method that only works for some customers. Whether someone pays by scanning a QR, tapping a wallet, or entering a card, it's the same clean, single-page checkout — which is what keeps the sale from dropping at the end.",
            "You pay only your gateway's rate. We don't add a platform fee on top, so what a UPI order costs you is your gateway's fee and nothing else \u2014 UPI itself carries no merchant discount rate.",
          ],
        },
        {
          heading: "A shop that looks like a brand",
          body: [
            "Indian D2C is crowded, and looking like everyone else is a quiet way to lose. Mark8ly's default storefront is editorial — considered typography, generous space, product pages that make each item look worth buying. You get that look without a design fee or a theme licence.",
            "Your own domain is included, so customers land on your brand rather than a marketplace listing or a platform subdomain. It reads as a real shop, because it is one.",
          ],
        },
        {
          heading: "The GST and gateway maths nobody puts on the pricing page",
          body: [
            "Two things make selling online in India cost more than the headline rate suggests, and neither appears in most comparisons.",
            "The first is that Shopify Payments is not available in India \u2014 Stripe, which powers it, has not launched here. Indian merchants therefore have to use a third-party gateway such as Razorpay, which is precisely the case where Shopify Basic adds its 2% platform fee. On a Shopify store in India that 2% is not an avoidable option; it is unavoidable, and it sits on top of your gateway's own rate.",
            "The second is GST. Payment gateway fees attract 18% GST, so a quoted 2% gateway rate costs 2.36% once tax is applied. If you are GST-registered you can generally claim that back as input tax credit, but it is still cash out of the account first, and it is rarely mentioned when a rate is quoted at you.",
          ],
          breakdown: {
            caption: "\u20b91,00,000 of monthly sales, Shopify Basic with a third-party gateway",
            rows: [
              { label: "Shopify platform fee", amount: "\u20b92,000", note: "2%, unavoidable \u2014 Shopify Payments is not available in India" },
              { label: "Gateway fee", amount: "\u20b92,000", note: "around 2%, varies by method and provider" },
              { label: "GST on the gateway fee", amount: "\u20b9360", note: "18%, reclaimable as input credit if you are registered" },
            ],
            total: { label: "Before the subscription", amount: "\u20b94,360", note: "The \u20b92,000 platform fee is the half that buys you nothing." },
            source: {
              label: "Shopify's published pricing",
              url: "https://www.shopify.com/pricing",
            },
          },
        },
        {
          heading: "UPI itself costs nothing, and that surprises people",
          body: [
            "The merchant discount rate on person-to-merchant UPI payments has been nil since January 2020, under Section 10A of the Payment and Settlement Systems Act, 2007 and Section 269SU of the Income-tax Act, 1961. Accepting UPI is not, in the regulated sense, a cost at all.",
            "What you pay is your gateway's own fee. Razorpay's published pricing states it plainly \u2014 \u201cZero MDR \u2014 2% platform fee applies\u201d \u2014 the same 2% it charges on cards and wallets. Knowing which of the two you are being charged is worth something, because only one of them is anyone's to negotiate.",
            "Worth watching: the Taxation and Other Laws (Amendment) Bill, 2026 amended the Act to allow MDR to be notified above a turnover threshold. Neither the rate nor the threshold has been set, and smaller merchants are expected to remain exempt, but it is no longer a settled zero.",
          ],
        },
        {
          heading: "Start today, keep everything",
          body: [
            "You don't need a developer. If you can write an email, you can open a store and be selling by the end of the afternoon — and real humans answer when you get stuck.",
            "Prices are shown in your own currency, so you always know what you'll pay. And it's your shop: export your products, customers, and orders in one click whenever you want. No lock-in.",
          ],
        },
      ]}
      faq={[
        {
          question: "Does Mark8ly support UPI payments?",
          answer:
            "Yes — UPI is built in as a first-class payment method, alongside wallets and cards, all behind a single checkout. There's nothing region-locked to work around.",
        },
        {
          question: "What does it cost to sell online in India?",
          answer:
            "There's no platform fee on your sales \u2014 you pay only your payment gateway's rate, typically around 2%. UPI's own merchant discount rate has been nil since 2020, so that 2% is the gateway's fee rather than a cost of UPI itself. After 90 days free, Starter is \u20b9999 a month (\u20b99,599 a year), Studio is \u20b92,499 and Pro is \u20b96,599.",
        },
        {
          question: "Can customers pay with cards and wallets too?",
          answer:
            "Yes. UPI, wallets, and cards all run through the same single-page checkout, so your customer can pay however they prefer.",
        },
        {
          question: "Do I need technical skills to set up?",
          answer:
            "No. Most sellers are live in an afternoon. If you can write an email, you can run a Mark8ly store, and real people help when you need it.",
        },
      ]}
      ctaHeading={
        <>
          Start selling
          <br />
          across India today.
        </>
      }
      ctaBody="UPI, wallets, and cards behind one checkout. No platform fees. Ninety days free, no card required."
    />
  );
}
