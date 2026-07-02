/**
 * Guides content — small set of honest, filler-free how-to articles for
 * new merchants. Data-driven so the index (/guides) and each article
 * (/guides/[slug]) render from one source, and the sitemap can enumerate
 * slugs without a second list.
 *
 * These target informational search intent ("how to start an online
 * store", "how to price handmade products", "accept UPI payments") and
 * link across to the commercial comparison pages and /onboarding.
 */

export type GuideBlock =
  | { type: "p"; text: string }
  | { type: "h2"; text: string }
  | { type: "ul"; items: ReadonlyArray<string> };

export interface Guide {
  slug: string;
  /** <title> — brand suffix is appended by the layout template. */
  title: string;
  description: string;
  eyebrow: string;
  heading: string;
  lede: string;
  /** ISO date, surfaced in Article JSON-LD + a "last updated" line. */
  updated: string;
  readingMinutes: number;
  blocks: ReadonlyArray<GuideBlock>;
}

export const GUIDES: ReadonlyArray<Guide> = [
  {
    slug: "how-to-start-an-online-store",
    title: "How to Start an Online Store (a Calm, Honest Guide)",
    description:
      "A step-by-step guide to starting an online store — from choosing what to sell to taking your first order — without the hype, upsells, or platform lock-in.",
    eyebrow: "Guide",
    heading: "How to start an online store",
    lede: "No hustle, no funnel jargon. Just the handful of decisions that actually matter, in the order they matter, so you can open a shop you're proud of by the end of the week.",
    updated: "2026-07-01",
    readingMinutes: 6,
    blocks: [
      {
        type: "p",
        text: "Most guides to starting an online store are really guides to buying things — themes, apps, courses, ads. You need very little of that. What you need is a clear product, a place to sell it, a way to get paid, and the patience to tell a few people. Here's the short version, honestly.",
      },
      { type: "h2", text: "1. Decide what you're actually selling" },
      {
        type: "p",
        text: "Start narrow. One product line you understand deeply beats a broad catalogue you can't photograph or describe well. Write down, in a sentence, who it's for and why they'd choose it over the obvious alternative. If you can't finish that sentence, the store won't fix it — the product will.",
      },
      {
        type: "p",
        text: "Sort out the boring-but-critical parts before you build anything: where stock is made or stored, how you'll pack it, and what shipping actually costs you. A beautiful storefront can't rescue economics that don't work.",
      },
      { type: "h2", text: "2. Choose where to sell" },
      {
        type: "p",
        text: "You have three broad options, and they're not mutually exclusive:",
      },
      {
        type: "ul",
        items: [
          "A marketplace (Etsy, Amazon): instant traffic, but you rent the customer, compete on price next to near-identical listings, and pay fees on every sale.",
          "A website builder / commerce platform: your own domain and brand, a flat monthly cost, and you own the customer relationship.",
          "Social selling (Instagram, WhatsApp): great for a first few orders, painful to scale — no real catalogue, checkout, or record-keeping.",
        ],
      },
      {
        type: "p",
        text: "Most people who are serious end up wanting their own site, because a brand customers remember is worth far more over time than a listing in someone else's mall. If you're weighing specific platforms, we've written plain comparisons for the Shopify alternative and the Etsy alternative questions.",
      },
      { type: "h2", text: "3. Set up the store" },
      {
        type: "p",
        text: "Whatever platform you pick, the setup is the same shape: add your products with honest photos and real descriptions, connect your own domain so customers land on your brand, and choose a storefront design that gets out of the way of the product. Resist the urge to add ten apps on day one — every extra tool is something to maintain.",
      },
      {
        type: "p",
        text: "A good rule: if a shopper can find a product, understand it, and buy it in under a minute, the store is done enough to open. Polish after you have real orders telling you what to polish.",
      },
      { type: "h2", text: "4. Get paid properly" },
      {
        type: "p",
        text: "Make sure checkout supports how your customers actually pay — cards, and where relevant local methods like UPI or wallets. The cost of taking money is your payment processor's fee (roughly 2% for UPI, 2–3% for cards). Watch for platforms that add their own percentage on top of that; over a year, that skim is real money out of a small margin.",
      },
      { type: "h2", text: "5. Tell people — then keep the ones who buy" },
      {
        type: "p",
        text: "Your first sales come from people who already know you: message them directly, don't run ads yet. Once orders trickle in, the highest-leverage thing you can do is keep the customer — collect emails, follow up, make a second purchase easy. Repeat buyers, not new traffic, are what turn a shop into a business.",
      },
      { type: "h2", text: "The short version" },
      {
        type: "p",
        text: "Pick one clear product. Sell it on your own site. Take payment cheaply. Tell people you already know. Keep the ones who buy. Everything else is optimisation you've earned the right to do later. You can open a Mark8ly store in an afternoon and start with exactly this.",
      },
    ],
  },
  {
    slug: "how-to-price-handmade-products",
    title: "How to Price Handmade Products Without Underselling Yourself",
    description:
      "A practical framework for pricing handmade products: cover materials and time, account for real costs and fees, and price for a margin that lets your craft survive.",
    eyebrow: "Guide",
    heading: "How to price handmade products",
    lede: "Most makers price too low, then wonder why every sale feels like a loss. Here's a straightforward way to set a number that respects your materials, your time, and the fact that you'd like to still be doing this next year.",
    updated: "2026-07-01",
    readingMinutes: 5,
    blocks: [
      {
        type: "p",
        text: "Pricing handmade work is hard because so much of the cost is invisible — your hours, your skill, the pieces that didn't work. If you only price the visible materials, you're funding the business out of your own wages. Here's a framework that puts the invisible costs back in.",
      },
      { type: "h2", text: "Start with your true costs" },
      {
        type: "p",
        text: "Add up everything a single item actually consumes:",
      },
      {
        type: "ul",
        items: [
          "Materials: what the physical inputs cost, including the offcuts and waste.",
          "Time: your hours to make it, paid at a rate you'd accept from anyone else.",
          "Overheads: a share of tools, studio space, packaging, and the sample or two that failed.",
          "Selling costs: payment processing (about 2–3%), platform fees, and shipping you absorb.",
        ],
      },
      {
        type: "p",
        text: "If a platform also takes a cut of each sale on top of processing, add that too — it's why makers on fee-heavy marketplaces quietly lose margin they never see. A platform with no per-sale fee keeps that money in the price you set.",
      },
      { type: "h2", text: "Apply a margin, not just a markup" },
      {
        type: "p",
        text: "Once you know the true cost per item, you need a margin on top — not to be greedy, but because the margin is what funds the next batch of materials, the slow months, and eventually paying yourself properly. A common maker approach is to at least double the true cost for a direct (retail) price. If that number makes you flinch, that's usually a sign you were underpricing, not that the price is wrong.",
      },
      { type: "h2", text: "Sanity-check against the market — then hold your line" },
      {
        type: "p",
        text: "Look at what comparable makers charge, but don't race them to the bottom. Handmade buyers are not only buying an object; they're buying the story, the quality, and the person behind it. Presentation matters here: the same piece photographed and described well on a considered storefront can hold a higher price than it could as a thumbnail in a crowded grid.",
      },
      { type: "h2", text: "Revisit it regularly" },
      {
        type: "p",
        text: "Prices aren't set once. As your skill grows, your materials cost more, or demand rises, your prices should move with them. Review them a few times a year. Undercharging isn't humility — it's a slow way to end a craft you love.",
      },
      {
        type: "p",
        text: "When you're ready to show your work at a price that respects it, Mark8ly is built for exactly this: an editorial storefront that makes a small, considered range look worth its price, with no platform fee eating into the margin you just worked out.",
      },
    ],
  },
  {
    slug: "accept-upi-payments-online",
    title: "How to Accept UPI Payments on Your Online Store",
    description:
      "A clear guide to accepting UPI payments online in India — how UPI works at checkout, what it costs, and how to offer it alongside cards and wallets without losing the sale.",
    eyebrow: "Guide",
    heading: "How to accept UPI payments online",
    lede: "In India, the sale is won or lost on the payment screen. Here's how UPI actually works for an online store, what it costs, and how to make sure a customer who wants to pay by UPI can — in one tap.",
    updated: "2026-07-01",
    readingMinutes: 5,
    blocks: [
      {
        type: "p",
        text: "For most Indian shoppers, UPI is the default way to pay — a quick scan or a tap, no card numbers, no friction. If your checkout treats UPI as an afterthought, you lose customers at the very last step, after all the hard work of getting them there. Making UPI first-class is one of the highest-return things an Indian store can do.",
      },
      { type: "h2", text: "How UPI works at checkout" },
      {
        type: "p",
        text: "At a high level, when a customer chooses UPI, they either scan a QR code or approve a payment request in their UPI app (GPay, PhonePe, Paytm, and others). The money moves bank-to-bank almost instantly, and your store gets confirmation. From the shopper's side it's two or three taps — which is exactly why it converts so well compared to typing card details.",
      },
      { type: "h2", text: "What it costs" },
      {
        type: "p",
        text: "UPI is one of the cheapest ways to take money online — processing is typically around 2%, and often lower than cards. The important thing to watch is whether your commerce platform adds its own fee on top of the processor's rate. That extra percentage is pure margin loss, and on Indian D2C margins it adds up fast. A platform with no per-sale platform fee means UPI costs you only what the processor charges.",
      },
      { type: "h2", text: "Offer UPI, wallets, and cards behind one checkout" },
      {
        type: "p",
        text: "Don't make customers choose a payment method before they've decided to buy, and don't force everyone through a single card processor. The best setup puts UPI, wallets, and cards behind one clean checkout, so whatever a shopper prefers just works:",
      },
      {
        type: "ul",
        items: [
          "UPI for the majority who pay by phone.",
          "Wallets for those with a balance they'd rather use.",
          "Cards for higher-value orders and customers who prefer them.",
        ],
      },
      {
        type: "p",
        text: "A checkout that quietly supports all three, on a single page, is what keeps the sale from dropping at the end.",
      },
      { type: "h2", text: "Get it right from day one" },
      {
        type: "p",
        text: "You don't need to be technical to accept UPI online. On Mark8ly, UPI, wallets, and cards are built into the checkout from the start — nothing region-locked to work around — and there's no platform fee on the sale. If you're selling to customers in India, that combination is the difference between a payment screen that converts and one that leaks.",
      },
      {
        type: "p",
        text: "For the fuller picture on selling to Indian customers — pricing in your own currency, your own domain, and a storefront that looks like a brand — see our guide to selling online in India.",
      },
    ],
  },
];

export function getGuide(slug: string): Guide | undefined {
  return GUIDES.find((g) => g.slug === slug);
}
