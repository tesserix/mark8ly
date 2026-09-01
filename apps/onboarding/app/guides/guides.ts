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
  {
    slug: "how-to-add-your-first-product",
    title: "How to Add Your First Product to Your Online Store",
    description:
      "A walkthrough of adding your first product in the Mark8ly admin — the fields that matter, when variants are worth the extra step, and what draft vs active actually controls.",
    eyebrow: "Guide",
    heading: "How to add your first product",
    lede: "The product form has more fields than any one product needs. Here's the order to fill them in, what's actually required, and what you can safely leave for later.",
    updated: "2026-09-02",
    readingMinutes: 6,
    blocks: [
      {
        type: "p",
        text: "A product listing only needs three things to save: a title, a price, and a stock count. Everything else on the form exists to handle products that are more complicated than yours might be. Here's what to fill in, in the order the form asks for it, and which parts you can ignore on day one.",
      },
      { type: "h2", text: "1. Open a new product" },
      {
        type: "p",
        text: "From Products in the admin sidebar, click New product. That opens a blank form at a URL like /products/new — nothing is saved yet, so there's no harm in clicking around before you commit to anything.",
      },
      { type: "h2", text: "2. Details: title, handle, description" },
      {
        type: "p",
        text: "Title is the only field on the whole form that's required by name. Handle — the part of the URL after /products/ — is optional and gets generated from the title automatically if you leave it blank, so don't overthink it unless you need a specific slug. Description sits below and shows up on the product page under the title; it's optional too, but an empty one looks unfinished to a shopper, so it's worth a paragraph even at this stage.",
      },
      { type: "h2", text: "3. Pricing and inventory" },
      {
        type: "p",
        text: "For a plain product with no variants, this section is three fields: price, stock, and SKU. Price needs a number like 19.99 — the form will reject anything else. Stock defaults to 0, and there's an \"Always in stock\" checkbox for products you don't want to run out on the storefront (digital goods, made-to-order pieces); stock still counts down on each order, it just never blocks a sale. SKU is optional — leave it blank and one gets generated from the handle.",
      },
      {
        type: "p",
        text: "If your store has more than one warehouse, per-warehouse stock only appears once the product exists — you'll set the overall stock number here first, then split it by location after saving.",
      },
      { type: "h2", text: "4. Options — only if this product comes in more than one version" },
      {
        type: "p",
        text: "Skip this section entirely if you're selling one version of one thing. If you're not — a T-shirt in three sizes, a candle in four scents — add an option (Size, Colour, whatever applies) and list its values. The moment you do, the Pricing and inventory section above turns into a variant table: one row per combination, each with its own price, stock, and SKU. Add a second option and the table fills in every combination automatically. There's no obligation to price every variant the same — that's the whole point of breaking it out.",
      },
      { type: "h2", text: "5. Shipping" },
      {
        type: "p",
        text: "Weight and dimensions here are what carrier rates get calculated from at checkout. If you skip this, the storefront falls back to a default envelope for the rate quote — fine for a quick test, worth fixing before you take real orders, because a wrong quote either loses you money on every sale or overcharges a customer who then doesn't come back.",
      },
      { type: "h2", text: "6. Categories and tax" },
      {
        type: "p",
        text: "Categories live in the right-hand rail and control where the product shows up when someone browses your storefront rather than searching it directly — worth setting even with only a handful of products, since \"browse everything\" stops being a real navigation option past about a dozen items. Tax fields below the main form adapt to your store's country; if you're not sure what applies, leave the default and revisit it with an accountant rather than guessing.",
      },
      { type: "h2", text: "7. Draft vs active — and photos" },
      {
        type: "p",
        text: "Every new product starts as a draft, set in the Status field in the same right-hand rail. A draft is saved but not visible on your storefront — switch it to Active when you're ready for customers to see it. One thing to know before you save: photos aren't on the create form at all. A product needs to exist first before you can attach media to it, so the Media section only appears once you've saved the product for the first time and are editing it. Save with the basics, then come straight back in to add photos before switching it to Active — a product page without a photo is not one you want live.",
      },
      {
        type: "p",
        text: "That's the whole flow: title, price, stock, save. Everything else — variants, shipping weights, categories — is there for when a product needs it, not before. Once your first product is live, adding the next one is the same three fields again.",
      },
    ],
  },
  {
    slug: "how-to-connect-your-domain",
    title: "How to Connect Your Domain to Your Storefront",
    description:
      "A step-by-step guide to pointing your own domain at your Mark8ly storefront — the DNS records to add, how ownership and SSL get verified, and how long it actually takes.",
    eyebrow: "Guide",
    heading: "How to connect your domain",
    lede: "A store on your own domain looks like a business. A store on someone else's subdomain looks like a trial. Here's exactly what to add at your DNS provider, and what each record is actually for.",
    updated: "2026-09-02",
    readingMinutes: 6,
    blocks: [
      {
        type: "p",
        text: "Connecting a domain is a DNS change, not a code change — you're telling the internet that your domain now points at your storefront. It takes a few minutes to set up and, depending on your DNS provider, anywhere from a few minutes to a couple of days to take effect. Here's the whole process, record by record.",
      },
      { type: "h2", text: "1. Start from Settings → Domains" },
      {
        type: "p",
        text: "In the admin, go to Settings, then Domains, and use Add a custom domain. You'll be asked for the domain itself and which of two setup methods to use: manual, where you add DNS records yourself, or Cloudflare, where an API token lets the platform create them for you automatically. Manual works with any registrar and is what most people use; Cloudflare is faster if that's already where your domain's DNS is hosted.",
      },
      { type: "h2", text: "2. Prove you own the domain" },
      {
        type: "p",
        text: "The first record is a TXT record, with a name and value generated specifically for your domain and shown on screen after you add it. Only someone with access to your DNS settings can publish a TXT record, which is exactly why it's used as proof of ownership before anything else happens. You can remove it once verification succeeds — it's not needed after that.",
      },
      { type: "h2", text: "3. Route traffic to your storefront" },
      {
        type: "p",
        text: "This is the record that actually makes the domain work, and there are two ways to do it — pick the one your DNS provider supports for your situation. The Domains screen shows the exact value for each, with a copy button; copy from there rather than retyping from anywhere else, including this page — these values are the kind of thing that can change, and the screen is always current:",
      },
      {
        type: "ul",
        items: [
          "Option A — an A record at your domain, pointing to the IP address shown on the Domains screen. Use this for an apex domain (example.com rather than shop.example.com) — most DNS providers don't allow a CNAME at the apex.",
          "Option B — a CNAME record at your domain, pointing to edge.mark8ly.com. Cleaner if you're using a subdomain and your provider supports it.",
        ],
      },
      { type: "h2", text: "4. Let SSL provision itself" },
      {
        type: "p",
        text: "A third record — a CNAME from _acme-challenge.yourdomain.com to _acme-challenge.yourdomain.com.acme.mark8ly.com — delegates certificate issuance back to Mark8ly. It's how you get a free, auto-renewing SSL certificate without ever handing over your DNS credentials. Add it once alongside the routing record; you won't need to touch it again after that.",
      },
      { type: "h2", text: "5. Optional: a branded admin subdomain" },
      {
        type: "p",
        text: "If you'd also like admin.yourdomain.com to work as a branded URL for your admin dashboard, add one more A record — admin.yourdomain.com pointing to the same IP address as the storefront record — and it's active alongside the rest. Skip this if the default admin URL is fine; it changes nothing about the storefront.",
      },
      { type: "h2", text: "6. Verify, and be patient with DNS" },
      {
        type: "p",
        text: "Once the records are added, click Verify. DNS changes can take up to 48 hours to propagate fully, though most providers are faster — a tool like dnschecker.org will tell you whether your records have gone live before you try verifying again. If verification fails, the error tells you what it found: a missing CNAME, an unproven TXT record, or a routing record pointing somewhere other than what's expected. Fix the specific thing it names and verify again — there's no need to redo records that already checked out.",
      },
      { type: "h2", text: "7. What \"active\" looks like" },
      {
        type: "p",
        text: "Once DNS verification succeeds, the domain's status moves to active and SSL certificate provisioning starts automatically — that can take a few minutes on its own, and a refresh action on the domain's card shows you when the certificate is live. From that point, your storefront answers on your own domain with a valid certificate, and you're done. No further DNS maintenance is needed unless you change registrars or move the domain elsewhere.",
      },
    ],
  },
];

export function getGuide(slug: string): Guide | undefined {
  return GUIDES.find((g) => g.slug === slug);
}
