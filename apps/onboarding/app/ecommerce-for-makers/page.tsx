import type { Metadata } from "next";

import { SeoLanding } from "@/components/marketing/SeoLanding";

export const metadata: Metadata = {
  title: "Ecommerce Platform for Makers & Handmade Sellers",
  description:
    "An ecommerce platform for makers and handmade sellers who want a storefront that looks designed, no platform fees, and 90 days free. Built for people who make things.",
  alternates: { canonical: "/ecommerce-for-makers" },
  openGraph: {
    title: "Ecommerce for makers — a storefront worth opening",
    description:
      "For handmade sellers and independent makers: an editorial storefront, no platform fees, and 90 days free.",
    url: "https://mark8ly.com/ecommerce-for-makers",
  },
};

export default function EcommerceForMakersPage() {
  return (
    <SeoLanding
      eyebrow="Ecommerce for makers"
      title={
        <>
          A storefront for people
          <br />
          who make things.
        </>
      }
      lede="Mark8ly is an ecommerce platform for makers and handmade sellers — an editorial storefront that shows your work properly, no platform fees on your sales, and ninety days to try it for free."
      intro={[
        "Most ecommerce platforms are built for catalogues, not craft. They optimise for thousands of SKUs, bulk imports, and dashboards full of metrics — which is exactly wrong for a maker with forty carefully-made products and a story behind each one. What you need is a shop that treats each item like it matters, not a spreadsheet with a checkout bolted on.",
        "Mark8ly is built the other way round. The default storefront is editorial: quiet typography, generous whitespace, and product pages that give a single object room to breathe. It's the kind of layout you'd normally hire a designer for — because we did, working alongside real merchants who sell the things they make.",
        "And because makers usually run on thin margins, we don't take a cut. There's no platform fee on your sales — you pay only your payment processor's standard rate. Ninety days free to start, then plans from $15 a month billed yearly, with unlimited products on every tier — including the Starter plan people open their first shop on.",
      ]}
      competitorName="Generic platforms"
      comparisonNote="Most mainstream platforms are tuned for high-SKU catalogues and add fees or design costs that hit small makers hardest."
      comparison={[
        { label: "Free to try", mark8ly: "90 days", them: "3–15 days" },
        {
          label: "Platform fee per sale",
          mark8ly: "None",
          them: "Often 2% or a per-sale cut",
        },
        {
          label: "Default storefront design",
          mark8ly: "Editorial, product-first",
          them: "Catalogue templates",
        },
        {
          label: "Good for small ranges",
          mark8ly: "Yes — each item gets room",
          them: "Optimised for bulk SKUs",
        },
        {
          label: "Design cost to look good",
          mark8ly: "None — designed by default",
          them: "Theme licence or design fee",
        },
        {
          label: "Own your data",
          mark8ly: "One-click export, any time",
          them: "Partial / CSV export",
        },
      ]}
      sections={[
        {
          heading: "Your work, shown properly",
          body: [
            "A handmade object deserves better than a thumbnail in a grid. Mark8ly's product pages are built to hold a single item — large imagery, considered type, space for the detail and the story that makes someone buy. The storefront reads as a shop you'd walk into, not an inbox you can't close.",
            "You don't need a designer or a theme marketplace to get there. The editorial look is the default, so a shop of forty pieces looks composed the moment it's live.",
          ],
        },
        {
          heading: "Margins that survive the sale",
          body: [
            "When you make things by hand, every percentage point matters. Mark8ly adds nothing to your sales — no platform fee, no per-order surcharge. Your gateway takes its standard rate (about 2% for UPI, 2–3% for cards) and that's it.",
            "The Starter plan is priced for a first shop and still carries unlimited products and orders — list a considered range or a deep archive, it costs the same. What you gain further up is more storefronts, more images per product, and deeper API access.",
          ],
        },
        {
          heading: "What a maker's storefront needs that a generic one doesn't",
          body: [
            "Most storefront software is built around a catalogue: many products, many variants, consistent stock, photography shot to a template. Handmade work is the opposite of nearly all of that, and the mismatch shows up as daily friction rather than as a missing feature.",
            "A maker often has one of a thing. Not low stock \u2014 one. A platform that treats a sold-out listing as an error state, or pushes you to restock, is describing a business you are not running. Made-to-order work has the reverse problem: no stock number is meaningful, and what the buyer needs to know is how long it will take.",
            "Variants are physical rather than a dropdown. Ring sizes, timber grain, dye lots and glaze results are not interchangeable options behind one photograph \u2014 the variation is frequently the reason someone is buying. A storefront that hides it behind a swatch is hiding the work.",
            "Photography is doing the job a customer's hands would do. Texture, scale and colour accuracy carry the sale, which is why generous, uncropped images matter more here than in most categories, and why a template that squeezes everything into a uniform square costs real money.",
            "And postage is genuinely hard: one-off dimensions, fragile pieces, and a real risk of quoting a flat rate that quietly eats the margin on the heaviest item.",
            "None of this is exotic. It is just a different shape of business from the one most ecommerce software was designed around, and it is the shape we built for.",
          ],
        },
        {
          heading: "Simple enough to run alone",
          body: [
            "The admin does one thing per screen: products, orders, customers, inventory. No dashboards full of numbers that don't matter to a maker. If you can write an email, you can run the shop — and when you get stuck, real humans answer.",
            "You can be selling by the end of the afternoon, and if it's ever not right for you, one-click export means your products, customers, and orders come with you. No lock-in, no hard feelings.",
          ],
        },
      ]}
      faq={[
        {
          question: "Is Mark8ly good for a small range of handmade products?",
          answer:
            "Yes — it's designed for it. Product pages give each item room, and the editorial storefront makes a small, considered range look composed without any design work from you.",
        },
        {
          question: "How much does it cost to sell handmade goods here?",
          answer:
            "There's no platform fee on your sales. You pay only your payment processor's standard rate. Plans start at $15 a month billed yearly after 90 days free, with a Starter tier for first-time shops.",
        },
        {
          question: "How many products can I list?",
          answer:
            "As many as you like — products and orders are unlimited on every plan, Starter included.",
        },
        {
          question: "Do I need to be technical?",
          answer:
            "No. If you can write an email, you can open and run a store on Mark8ly, usually in an afternoon. Real people help when you need it.",
        },
      ]}
      ctaHeading={
        <>
          Show your work
          <br />
          the way it deserves.
        </>
      }
      ctaBody="Ninety days free, no card required, and no platform fee on a single sale. Open your maker's shop this afternoon."
    />
  );
}
