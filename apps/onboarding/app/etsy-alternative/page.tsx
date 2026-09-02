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
        "There's no per-sale platform fee. You pay only your payment processor's standard rate; we don't add a cut on top. Ninety days free to start, then plans from $15 a month billed yearly — a flat, predictable cost instead of a stack of fees that grows with every order.",
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
          heading: "What Etsy actually costs, on one order",
          body: [
            "Etsy's fees are individually small and collectively not. The listing fee is twenty cents, the transaction fee is 6.5%, and payment processing in the US is 3% plus twenty-five cents. The detail that catches people out is that the 6.5% applies to the total order including shipping \u2014 so charging the buyer for postage also increases what Etsy takes.",
            "Here is a $53 order: a $45 item with $8 of shipping.",
          ],
          breakdown: {
            caption: "A $53 order \u2014 a $45 item plus $8 shipping, US seller",
            rows: [
              { label: "Listing fee", amount: "$0.20", note: "per listing, renewed every four months or on each sale" },
              { label: "Transaction fee", amount: "$3.45", note: "6.5% of $53 \u2014 charged on the shipping too" },
              { label: "Payment processing", amount: "$1.84", note: "3% + $0.25, US rate" },
            ],
            total: { label: "Etsy keeps", amount: "$5.49", note: "10.4% of the order, before any advertising" },
            source: {
              label: "Etsy's published seller fees",
              url: "https://www.etsy.com/legal/fees/",
            },
          },
        },
        {
          heading: "And that is the version without Offsite Ads",
          body: [
            "Offsite Ads add 15% of the order on any sale that follows an Etsy-placed ad. On the same $53 order that is a further $7.95, taking the total to $13.44 \u2014 just over a quarter of the order.",
            "The part worth reading carefully: opting out is only available while your shop is under $10,000 of sales in a rolling twelve months. Cross that line once and enrolment becomes permanent for the life of the shop, at a reduced 12% rate capped at $100 per order. It does not lapse if your sales later fall back below the threshold.",
            "None of this makes Etsy a bad place to sell. It has demand you would otherwise have to go and find, and for many makers that trade is worth it. But it is a marketplace fee, and it scales with your success rather than with anything it costs Etsy to serve you.",
            "On Mark8ly there is no listing fee, no transaction fee, and no advertising levy. You pay your payment processor \u2014 on that same $53 order, around $1.84, essentially identical to Etsy's own processing charge \u2014 and nothing else. The $3.65 difference per order is the whole of it. At forty orders a month that is roughly $146 you keep, against $15 a month for Starter billed yearly.",
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
            "No. There are no listing fees and no platform transaction fee. You pay only your payment processor's standard rate, plus a flat plan starting at $15 a month billed yearly after 90 days free.",
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
