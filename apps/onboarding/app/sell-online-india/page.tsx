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
        "And on top of that, we take nothing from your sales. There's no platform fee — you pay only your payment processor's standard rate (around 2% for UPI). For Indian sellers running on real margins, keeping that 2% platform skim in your own pocket is decisive. Ninety days free to start, then ₹999 a month for Starter — or ₹9,599 for the year, which works out at about ₹800 a month.",
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
            "You pay only your processor's standard rate. We don't add a platform fee on top, so the cost of taking a UPI payment is the cost of the UPI payment — nothing else.",
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
            "There's no platform fee on your sales — you pay only your payment processor's standard rate, around 2% for UPI. After 90 days free, Starter is ₹999 a month (₹9,599 a year), Studio is ₹2,499 and Pro is ₹6,599.",
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
