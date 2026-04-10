# Dashboard D3 — Help Center Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ship a markdown-based help center with ~14 articles, client-side search, category grid, and contextual HelpLink components across settings pages.

**Architecture:** Markdown files in `apps/admin/content/help/`. Server-side rendering via marked or next-mdx-remote. No migration, no backend — pure frontend.

**Tech Stack:** Next.js 16, React 19, Tailwind, marked (markdown rendering).

---

## Decisions Locked

1. **Markdown rendering:** Use `marked` (lightweight, no MDX needed) + `DOMPurify` for sanitization. No MDX components — plain markdown is sufficient for help articles.

2. **Content format:** Each `.md` file has YAML frontmatter with `title`, `category`, and `order` fields. Slug is derived from filename (e.g., `payments.md` -> `/support/help/payments`).

3. **Categories:** 5 fixed categories — "Getting Started", "Store Setup", "Operations", "Marketing", "Troubleshooting". Category is a frontmatter field, not a directory structure.

4. **Search:** Client-side filter on article title and first 200 characters of content. No backend search endpoint. Load all article metadata at build time via a helper that reads the content directory.

5. **"Was this helpful?"** feedback buttons are UI-only — no persistence. They show a "Thank you" message on click but do not store the response anywhere.

6. **HelpLink component:** A single-line component that renders `<a href="/support/help/{slug}">Learn more →</a>` with moss-700 color and reduced opacity.

---

## File Structure

### New files

```
apps/admin/
├── content/help/
│   ├── getting-started.md
│   ├── products.md
│   ├── orders.md
│   ├── payments.md
│   ├── shipping.md
│   ├── tax.md
│   ├── customers.md
│   ├── reviews.md
│   ├── marketing.md
│   ├── domains.md
│   ├── team.md
│   ├── subscription.md
│   ├── storefront.md
│   └── troubleshooting.md
├── lib/help.ts                           # Article loader + metadata parser
├── app/support/help/
│   ├── page.tsx                          # Help center landing (server component)
│   └── [slug]/
│       └── page.tsx                      # Article page (server component)
├── components/support/
│   ├── HelpCategoryGrid.tsx              # Category cards grid
│   ├── HelpArticleList.tsx               # Featured/filtered article list
│   ├── HelpSearch.tsx                    # Client-side search input
│   ├── HelpArticleRenderer.tsx           # Markdown-to-HTML renderer
│   ├── HelpArticleFeedback.tsx           # "Was this helpful?" buttons
│   └── HelpLink.tsx                      # Contextual "Learn more →" link
```

### Modified files

```
apps/admin/app/settings/payments/page.tsx     # Add HelpLink
apps/admin/app/settings/shipping/page.tsx     # Add HelpLink
apps/admin/app/settings/tax/page.tsx          # Add HelpLink
apps/admin/app/products/page.tsx              # Add HelpLink
apps/admin/app/orders/page.tsx                # Add HelpLink (if exists)
apps/admin/package.json                       # Add marked + @types/dompurify
```

---

## Tasks

### Task 1: Install dependencies

Add `marked` and `dompurify` to the admin app.

```bash
cd apps/admin
npm install marked dompurify
npm install -D @types/dompurify
```

**Verification:**
- [ ] `npm ls marked` shows marked installed
- [ ] `npm ls dompurify` shows dompurify installed
- [ ] `npx tsc --noEmit` still passes

---

### Task 2: Create markdown content files

Create all 14 help articles in `apps/admin/content/help/`.

**File: `apps/admin/content/help/getting-started.md`**

```markdown
---
title: "Getting Started with Mark8ly"
category: "Getting Started"
order: 1
---

# Getting Started with Mark8ly

Welcome to Mark8ly. This guide walks you through the essential steps to launch your online store.

## 1. Create Your Store

After signing up, your first step is creating a store. Navigate to **Settings > Stores** and click **Create Store**. Enter your store name and a unique slug that will become your storefront URL.

## 2. Add Your First Product

Go to **Products** and click **New Product**. Fill in the title, description, and price. You can add images, variants (like sizes or colors), and organize products into categories later.

## 3. Configure Payments

Head to **Settings > Payments** to connect a payment provider. Mark8ly supports Stripe and Razorpay. You will need your provider's API keys to complete the setup.

## 4. Set Up Shipping

Navigate to **Settings > Shipping** to configure shipping carriers and rates. You can set flat rates, weight-based rates, or connect to carrier APIs for real-time rates.

## 5. Configure Tax

Go to **Settings > Tax** to set up tax collection. Mark8ly supports automatic tax calculation via TaxJar integration, or you can configure manual tax rates per region.

## 6. Launch Your Storefront

Once payments and shipping are configured, your storefront is live at your store's URL. Share it with customers and start selling.

## Next Steps

- [Set up a custom domain](/support/help/domains)
- [Customize your storefront theme](/support/help/storefront)
- [Learn about order management](/support/help/orders)
```

**File: `apps/admin/content/help/products.md`**

```markdown
---
title: "Managing Products"
category: "Operations"
order: 6
---

# Managing Products

Products are the core of your store. This guide covers creating, editing, and organizing your product catalog.

## Creating a Product

Navigate to **Products > New Product**. Required fields are:

- **Title** — the product name displayed to customers
- **Price** — the selling price (set per variant if you have multiple options)
- **Status** — Draft (invisible to customers), Active (visible), or Archived

## Product Variants

If your product comes in different sizes, colors, or configurations, add options under the Variants section. Each combination of options creates a variant with its own price, SKU, and inventory count.

## Product Images

Upload images via the Media section. The first image becomes the primary image shown in listings. Supported formats are JPEG, PNG, and WebP. Images are automatically optimized for web delivery.

## Categories

Organize products into categories from the product edit page. Categories help customers navigate your storefront and can be nested up to 3 levels deep.

## Bulk Actions

Select multiple products from the list to perform bulk actions: activate, archive, delete, or change category.

## CSV Import & Export

For large catalogs, use **Products > Export CSV** to download your catalog, or import products via CSV upload. The CSV format includes columns for title, description, price, SKU, inventory, and category.

## Tips

- Use descriptive titles that include keywords customers might search for
- Write detailed descriptions — they help with search engine visibility
- Keep inventory counts accurate to avoid overselling
- Use high-quality images with consistent dimensions
```

**File: `apps/admin/content/help/orders.md`**

```markdown
---
title: "Order Management"
category: "Operations"
order: 7
---

# Order Management

This guide explains how to view, process, and manage customer orders.

## Order Lifecycle

Every order follows this flow:

1. **Pending** — order placed, awaiting payment confirmation
2. **Confirmed** — payment received, ready to process
3. **Fulfilled** — items shipped or delivered
4. **Cancelled** — order cancelled (before fulfillment only)

## Viewing Orders

Navigate to **Orders > All Orders** to see all orders. Use the status tabs to filter by lifecycle stage. Click any order to see its full details including line items, customer information, and payment status.

## Processing Orders

When an order is confirmed:

1. Review the line items and shipping address
2. Pack the items
3. Click **Mark Fulfilled** and optionally add tracking information
4. The customer receives a fulfillment notification

## Cancellations and Refunds

- **Cancel:** Available for pending or confirmed orders. Cancelling an order with a completed payment triggers a refund.
- **Refund:** Issue a full or partial refund from the order detail page. The refund is processed through the original payment provider.

## Returns

Customers can request returns from their order confirmation page. Navigate to **Orders > Returns & Refunds** to manage return requests. Approve or reject returns, mark items as received, and process refunds.

## Abandoned Carts

View incomplete checkouts under **Orders > Abandoned Carts**. You can trigger recovery emails to remind customers about their unpurchased items.
```

**File: `apps/admin/content/help/payments.md`**

```markdown
---
title: "Setting Up Payments"
category: "Store Setup"
order: 3
---

# Setting Up Payments

Mark8ly supports multiple payment providers to process customer transactions securely.

## Supported Providers

- **Stripe** — credit/debit cards, Apple Pay, Google Pay (recommended for most stores)
- **Razorpay** — popular in India, supports UPI, netbanking, and cards

## Connecting Stripe

1. Go to **Settings > Payments**
2. Click **Configure** next to Stripe
3. Enter your Stripe API keys (found in your Stripe Dashboard under Developers > API Keys)
4. Use **test keys** during setup, switch to **live keys** when ready to accept real payments
5. Click **Test Connection** to verify the keys work
6. Save the configuration

## Connecting Razorpay

1. Go to **Settings > Payments**
2. Click **Configure** next to Razorpay
3. Enter your Razorpay Key ID and Key Secret
4. Click **Test Connection** to verify
5. Save the configuration

## Test Mode vs Live Mode

Always test your checkout flow with test keys before going live:

- **Stripe test card:** 4242 4242 4242 4242 (any future expiry, any CVC)
- **Razorpay test mode:** Use the test dashboard credentials

## Currency

Payment currency is set at the store level and must match your payment provider's supported currencies. Most stores use a single currency.

## Troubleshooting

- **"Invalid API key"** — double-check that you are using the correct key type (test vs live)
- **"Connection failed"** — ensure your provider account is active and not restricted
- **Payments not appearing** — verify you are using live keys in production
```

**File: `apps/admin/content/help/shipping.md`**

```markdown
---
title: "Configuring Shipping"
category: "Store Setup"
order: 4
---

# Configuring Shipping

Set up shipping to define how products are delivered to your customers.

## Shipping Providers

Mark8ly integrates with shipping carriers to provide real-time rates and tracking:

- **Shiprocket** — multi-carrier aggregator popular in India
- **Manual rates** — define flat or weight-based rates yourself

## Setting Up Shipping Rates

1. Go to **Settings > Shipping**
2. Choose your shipping method:
   - **Flat rate** — single price for all orders
   - **Weight-based** — rates calculated by total order weight
   - **Carrier-calculated** — real-time rates from your connected carrier
3. Configure shipping zones (regions you ship to)
4. Set a free shipping threshold if desired

## Connecting Shiprocket

1. Create a Shiprocket account
2. Enter your Shiprocket API credentials in **Settings > Shipping**
3. Select your preferred carriers within Shiprocket
4. Test with a sample shipment

## Shipping Zones

Shipping zones let you set different rates for different regions. Common zones:

- **Domestic** — within your country
- **Regional** — neighboring countries
- **International** — worldwide

## Free Shipping

Offer free shipping above a minimum order value to increase average order size. Configure this in your shipping settings or create a free shipping coupon.
```

**File: `apps/admin/content/help/tax.md`**

```markdown
---
title: "Tax Configuration"
category: "Store Setup"
order: 5
---

# Tax Configuration

Proper tax setup ensures you collect and report the correct tax amounts on orders.

## Automatic Tax Calculation

Mark8ly integrates with **TaxJar** for automatic tax calculation based on the customer's shipping address. TaxJar handles:

- Sales tax rates by jurisdiction (US states, cities, counties)
- GST/VAT for international orders
- Tax-exempt product categories
- Nexus determination

## Setting Up TaxJar

1. Go to **Settings > Tax**
2. Click **Configure TaxJar**
3. Enter your TaxJar API token (found in your TaxJar dashboard)
4. Save the configuration

TaxJar will automatically calculate the correct tax for each order based on:
- Your business nexus locations
- The customer's shipping address
- The product type

## Manual Tax Setup

If you prefer not to use TaxJar, you can configure manual tax rates:

- Set a default tax rate for your store
- Override rates for specific regions
- Mark certain products as tax-exempt

## Tax Reports

Tax collected on orders is visible in each order's detail view. For filing purposes, export your orders and calculate totals by jurisdiction.

## Common Questions

- **Do I need to collect tax?** — This depends on your business location and where your customers are. Consult a tax professional.
- **What about international orders?** — TaxJar handles VAT/GST for supported countries. For others, consider duties-and-taxes-inclusive pricing.
```

**File: `apps/admin/content/help/customers.md`**

```markdown
---
title: "Managing Customers"
category: "Operations"
order: 8
---

# Managing Customers

Track your customer base and understand buying patterns.

## Customer Profiles

Navigate to **Customers > All Customers** to see everyone who has placed an order. Each profile shows:

- Contact information (name, email)
- Order history and total spend
- Account creation date

## Customer Accounts

Customers can create accounts on your storefront to:

- Save their shipping addresses
- View order history
- Track shipments
- Leave product reviews

Account creation is optional — customers can also check out as guests.

## Customer Search

Use the search bar to find customers by name or email. This is useful when handling support requests or looking up order details.
```

**File: `apps/admin/content/help/reviews.md`**

```markdown
---
title: "Product Reviews"
category: "Operations"
order: 9
---

# Product Reviews

Reviews build trust and help customers make purchasing decisions.

## How Reviews Work

After receiving an order, customers can leave a review on any product they purchased. Reviews include:

- A star rating (1-5)
- Written feedback
- The customer's name

## Moderating Reviews

Navigate to **Customers > Reviews** to see all reviews. You can:

- **Approve** reviews to make them visible on the storefront
- **Reject** reviews that violate your policies
- View pending reviews that need moderation

## Review Display

Approved reviews appear on the product page in your storefront. The average rating is shown in product listings and search results.

## Tips

- Respond to negative reviews professionally — it shows you care about customer satisfaction
- Encourage reviews by following up after delivery
- Never fake reviews — customers can tell, and it damages trust
```

**File: `apps/admin/content/help/marketing.md`**

```markdown
---
title: "Marketing Tools"
category: "Marketing"
order: 10
---

# Marketing Tools

Mark8ly provides several tools to help you attract and retain customers.

## Coupons

Create discount codes under **Marketing > Coupons**:

- **Percentage discounts** — e.g., 15% off
- **Fixed amount discounts** — e.g., $10 off
- **Free shipping** — waive shipping costs

Set usage limits, minimum purchase requirements, and expiration dates to control your promotions.

## Gift Cards

Sell digital gift cards under **Marketing > Gift Cards**. Customers purchase them as gifts, and recipients redeem them at checkout.

## Loyalty Program

Reward repeat customers with points under **Marketing > Loyalty**. Customers earn points on purchases and redeem them for discounts on future orders.

## Campaigns

Plan and track marketing campaigns under **Marketing > Campaigns**. Link campaigns to specific coupons or promotions to measure their effectiveness.
```

**File: `apps/admin/content/help/domains.md`**

```markdown
---
title: "Custom Domains"
category: "Store Setup"
order: 11
---

# Custom Domains

Connect your own domain name to give your store a professional, branded URL.

## How It Works

By default, your store is accessible at `yourstore.mark8ly.com`. With a custom domain, customers visit `yourstore.com` instead.

## Connecting a Domain

1. Go to **Settings > Domains**
2. Click **Add Domain**
3. Enter your domain name (e.g., `shop.yourbrand.com`)
4. Add the DNS records shown to your domain registrar:
   - A CNAME record pointing to Mark8ly's servers
5. Wait for DNS propagation (usually 15-30 minutes, can take up to 48 hours)
6. Mark8ly automatically provisions an SSL certificate

## DNS Configuration

| Record Type | Name | Value |
|-------------|------|-------|
| CNAME | shop | `proxy.mark8ly.com` |

If you are connecting a root domain (e.g., `yourbrand.com` without a subdomain), use your registrar's ALIAS or ANAME record, or contact their support for root domain CNAME support.

## SSL Certificates

SSL certificates are automatically provisioned and renewed for all custom domains. No action required on your part.

## Troubleshooting

- **Domain not resolving** — check that DNS records are correct and wait for propagation
- **SSL error** — ensure no conflicting SSL settings at your registrar or CDN
- **Domain already in use** — each domain can only be connected to one store
```

**File: `apps/admin/content/help/team.md`**

```markdown
---
title: "Team Management"
category: "Store Setup"
order: 12
---

# Team Management

Invite team members to help manage your store with appropriate access levels.

## Roles

Mark8ly has three roles:

- **Owner** — full access including billing, domain, and team management
- **Admin** — can manage products, orders, customers, and most settings
- **Staff** — read-only access to products and orders, can process orders

## Inviting Team Members

1. Go to **Settings > Team**
2. Click **Invite Member**
3. Enter the person's email address
4. Select their role
5. They receive an email invitation to join

## Managing Members

From the Team settings page, you can:

- Change a member's role
- Remove a member from the store
- View pending invitations

## Best Practices

- Use the **Staff** role for warehouse or fulfillment team members who only need to view and process orders
- Reserve **Owner** access for business principals only
- Review team access regularly and remove inactive members
```

**File: `apps/admin/content/help/subscription.md`**

```markdown
---
title: "Subscription & Billing"
category: "Store Setup"
order: 13
---

# Subscription & Billing

Manage your Mark8ly subscription and billing details.

## Plans

Mark8ly offers tiered plans based on your store's needs:

- **Starter** — for new stores getting started
- **Growth** — for growing businesses with more products and traffic
- **Scale** — for established stores with high volume

## Managing Your Subscription

Go to **Settings > Subscription** to:

- View your current plan and usage
- Upgrade or downgrade your plan
- Update payment method
- View billing history and download invoices

## Billing Cycle

Subscriptions are billed monthly. Changes take effect at the start of the next billing cycle. Upgrades are prorated — you only pay the difference for the remaining days in the current cycle.

## Cancellation

You can cancel your subscription at any time. Your store remains active until the end of the current billing period. After that, your store enters read-only mode — you can export your data but customers cannot place new orders.
```

**File: `apps/admin/content/help/storefront.md`**

```markdown
---
title: "Storefront Customization"
category: "Store Setup"
order: 14
---

# Storefront Customization

Customize how your store looks to customers.

## Theme Settings

Navigate to **Settings > Storefront** to configure your storefront's appearance:

- **Logo** — upload your store logo (displayed in the header and emails)
- **Colors** — adjust accent colors to match your brand
- **Typography** — choose from available font pairings
- **Layout** — configure homepage sections and navigation

## Homepage Sections

Your storefront homepage can display:

- **Featured products** — hand-picked products to highlight
- **New arrivals** — automatically shows your latest products
- **Categories** — grid of product categories for navigation
- **Banner** — hero image with text overlay and call-to-action

## Navigation

Configure your storefront's navigation menu to help customers find products:

- Add links to categories, collections, or custom pages
- Organize into dropdown menus for complex catalogs
- Add external links (social media, blog, etc.)

## Tips

- Keep your homepage clean and focused — highlight 3-5 key products
- Use high-quality banner images that represent your brand
- Test your storefront on mobile — most shoppers browse on phones
```

**File: `apps/admin/content/help/troubleshooting.md`**

```markdown
---
title: "Troubleshooting"
category: "Troubleshooting"
order: 15
---

# Troubleshooting

Common issues and how to resolve them.

## Orders

### "Payment failed" on checkout
- Verify your payment provider API keys are correct and using live keys
- Check that the customer's card is valid and has sufficient funds
- Review the payment provider's dashboard for detailed error messages

### Orders not appearing
- Ensure the checkout completed successfully (check for webhook delivery in your payment provider dashboard)
- Verify the storefront is connected to the correct store

## Products

### Products not showing on storefront
- Check that the product status is **Active** (Draft products are hidden)
- Verify the product has at least one variant with a price
- Ensure the product has inventory in stock (if inventory tracking is enabled)

### Images not loading
- Verify image files are under 10MB
- Supported formats: JPEG, PNG, WebP
- Try re-uploading the image

## Payments

### "Connection failed" when testing payment provider
- Double-check API keys (no extra spaces)
- Ensure your provider account is active and not in restricted mode
- Verify you are using the correct key type (test vs live)

## Domains

### Custom domain not working
- Verify DNS records are correctly configured
- Allow up to 48 hours for DNS propagation
- Check for conflicting DNS records at your registrar

## Account

### Can't log in
- Use the "Forgot password" link on the login page
- Ensure you are using the correct email address
- Check for typos in your password
- Clear browser cookies and try again

## Still need help?

If you cannot find the answer here, [create a support ticket](/support/tickets/new) and our team will assist you.
```

**Verification:**
- [ ] All 14 `.md` files exist in `apps/admin/content/help/`
- [ ] Each file has valid YAML frontmatter with `title`, `category`, and `order`
- [ ] Content is practical, actionable, and references actual admin UI paths

---

### Task 3: Article loader utility

Create `apps/admin/lib/help.ts` — reads markdown files, parses frontmatter, and provides article metadata.

**File: `apps/admin/lib/help.ts`**

```typescript
import fs from "fs";
import path from "path";
import matter from "gray-matter";

/** Metadata parsed from article frontmatter. */
export interface HelpArticleMeta {
  slug: string;
  title: string;
  category: string;
  order: number;
  excerpt: string; // first 200 chars of content (stripped of markdown)
}

/** Full article with rendered content. */
export interface HelpArticle extends HelpArticleMeta {
  content: string; // raw markdown body (frontmatter stripped)
}

const CONTENT_DIR = path.join(process.cwd(), "content", "help");

/**
 * Returns metadata for all help articles, sorted by order.
 * Called at build/request time in server components.
 */
export function getAllArticles(): HelpArticleMeta[] {
  const files = fs.readdirSync(CONTENT_DIR).filter((f) => f.endsWith(".md"));

  const articles: HelpArticleMeta[] = files.map((filename) => {
    const slug = filename.replace(/\.md$/, "");
    const raw = fs.readFileSync(path.join(CONTENT_DIR, filename), "utf-8");
    const { data, content } = matter(raw);

    // Strip markdown syntax for excerpt.
    const plainText = content
      .replace(/#{1,6}\s+/g, "")
      .replace(/\*\*([^*]+)\*\*/g, "$1")
      .replace(/\[([^\]]+)\]\([^)]+\)/g, "$1")
      .replace(/[`*_~]/g, "")
      .replace(/\n+/g, " ")
      .trim();

    return {
      slug,
      title: (data.title as string) ?? slug,
      category: (data.category as string) ?? "Uncategorized",
      order: (data.order as number) ?? 99,
      excerpt: plainText.slice(0, 200),
    };
  });

  return articles.sort((a, b) => a.order - b.order);
}

/**
 * Returns a single article by slug, or null if not found.
 */
export function getArticleBySlug(slug: string): HelpArticle | null {
  const filename = `${slug}.md`;
  const filepath = path.join(CONTENT_DIR, filename);

  if (!fs.existsSync(filepath)) {
    return null;
  }

  const raw = fs.readFileSync(filepath, "utf-8");
  const { data, content } = matter(raw);

  const plainText = content
    .replace(/#{1,6}\s+/g, "")
    .replace(/\*\*([^*]+)\*\*/g, "$1")
    .replace(/\[([^\]]+)\]\([^)]+\)/g, "$1")
    .replace(/[`*_~]/g, "")
    .replace(/\n+/g, " ")
    .trim();

  return {
    slug,
    title: (data.title as string) ?? slug,
    category: (data.category as string) ?? "Uncategorized",
    order: (data.order as number) ?? 99,
    excerpt: plainText.slice(0, 200),
    content,
  };
}

/** All known categories with their display order. */
export const HELP_CATEGORIES = [
  "Getting Started",
  "Store Setup",
  "Operations",
  "Marketing",
  "Troubleshooting",
] as const;

/**
 * Groups articles by category. Returns an array of category objects
 * with their articles, maintaining category display order.
 */
export function getArticlesByCategory(): Array<{
  category: string;
  articles: HelpArticleMeta[];
}> {
  const all = getAllArticles();
  return HELP_CATEGORIES.map((cat) => ({
    category: cat,
    articles: all.filter((a) => a.category === cat),
  }));
}
```

Also install `gray-matter` for frontmatter parsing:

```bash
cd apps/admin
npm install gray-matter
```

**Verification:**
- [ ] `npx tsc --noEmit` passes
- [ ] `getAllArticles()` returns 14 articles sorted by order
- [ ] `getArticleBySlug("payments")` returns the payments article
- [ ] `getArticleBySlug("nonexistent")` returns null

---

### Task 4: Help center landing page

**File: `apps/admin/app/support/help/page.tsx`**

```tsx
import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { getAllArticles, getArticlesByCategory } from "@/lib/help";
import { HelpCategoryGrid } from "@/components/support/HelpCategoryGrid";
import { HelpArticleList } from "@/components/support/HelpArticleList";
import { HelpSearch } from "@/components/support/HelpSearch";

export default async function HelpCenterPage() {
  const session = await getServerSessionContext();
  const { tenantName, email } = session;

  const allArticles = getAllArticles();
  const byCategory = getArticlesByCategory();

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="mx-auto max-w-4xl px-8 py-8">
        <div className="flex flex-col gap-10">
          {/* Header */}
          <div className="flex flex-col gap-4">
            <h1 className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-4xl text-[color:var(--ink-900)]">
              Help Center
            </h1>
            <p className="text-sm text-[color:var(--ink-900)]/60">
              Find answers to common questions about managing your Mark8ly store.
            </p>
          </div>

          {/* Search */}
          <HelpSearch articles={allArticles} />

          {/* Category grid */}
          <HelpCategoryGrid categories={byCategory} />

          {/* Featured articles */}
          <div className="flex flex-col gap-4">
            <h2 className="text-sm font-semibold uppercase tracking-[0.08em] text-[color:var(--ink-900)]/40">
              All Articles
            </h2>
            <HelpArticleList articles={allArticles} />
          </div>
        </div>
      </main>
    </AdminShell>
  );
}
```

**File: `apps/admin/components/support/HelpCategoryGrid.tsx`**

```tsx
import Link from "next/link";
import { BookOpen, Settings, ShoppingBag, Megaphone, AlertTriangle } from "lucide-react";
import type { HelpArticleMeta } from "@/lib/help";

interface HelpCategoryGridProps {
  categories: Array<{
    category: string;
    articles: HelpArticleMeta[];
  }>;
}

const CATEGORY_ICONS: Record<string, typeof BookOpen> = {
  "Getting Started": BookOpen,
  "Store Setup": Settings,
  "Operations": ShoppingBag,
  "Marketing": Megaphone,
  "Troubleshooting": AlertTriangle,
};

export function HelpCategoryGrid({ categories }: HelpCategoryGridProps) {
  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {categories.map(({ category, articles }) => {
        if (articles.length === 0) return null;
        const Icon = CATEGORY_ICONS[category] ?? BookOpen;
        // Link to first article in category.
        const firstSlug = articles[0].slug;

        return (
          <Link
            key={category}
            href={`/support/help/${firstSlug}`}
            className="group flex flex-col gap-3 rounded-md border border-[color:var(--ink-900)]/[0.06] bg-white p-6 transition-colors hover:border-[color:var(--moss-700)]/30"
          >
            <Icon className="h-5 w-5 text-[color:var(--moss-700)]" aria-hidden="true" />
            <div>
              <h3 className="text-sm font-semibold text-[color:var(--ink-900)] group-hover:text-[color:var(--moss-700)]">
                {category}
              </h3>
              <p className="mt-1 text-xs text-[color:var(--ink-900)]/40">
                {articles.length} {articles.length === 1 ? "article" : "articles"}
              </p>
            </div>
          </Link>
        );
      })}
    </div>
  );
}
```

**File: `apps/admin/components/support/HelpArticleList.tsx`**

```tsx
import Link from "next/link";
import type { HelpArticleMeta } from "@/lib/help";

interface HelpArticleListProps {
  articles: HelpArticleMeta[];
}

export function HelpArticleList({ articles }: HelpArticleListProps) {
  return (
    <div className="divide-y divide-[color:var(--ink-900)]/[0.06]">
      {articles.map((article) => (
        <Link
          key={article.slug}
          href={`/support/help/${article.slug}`}
          className="flex flex-col gap-1 py-4 transition-opacity hover:opacity-80"
        >
          <div className="flex items-center gap-3">
            <h3 className="text-sm font-medium text-[color:var(--ink-900)]">
              {article.title}
            </h3>
            <span className="text-[10px] font-medium text-[color:var(--ink-900)]/30">
              {article.category}
            </span>
          </div>
          <p className="text-xs text-[color:var(--ink-900)]/50 line-clamp-2">
            {article.excerpt}
          </p>
        </Link>
      ))}
    </div>
  );
}
```

**File: `apps/admin/components/support/HelpSearch.tsx`**

```tsx
"use client";

import { useState, useMemo } from "react";
import Link from "next/link";
import { Search } from "lucide-react";
import type { HelpArticleMeta } from "@/lib/help";

interface HelpSearchProps {
  articles: HelpArticleMeta[];
}

export function HelpSearch({ articles }: HelpSearchProps) {
  const [query, setQuery] = useState("");

  const results = useMemo(() => {
    if (!query.trim()) return [];
    const lower = query.toLowerCase();
    return articles.filter(
      (a) =>
        a.title.toLowerCase().includes(lower) ||
        a.excerpt.toLowerCase().includes(lower),
    );
  }, [query, articles]);

  const showResults = query.trim().length > 0;

  return (
    <div className="relative">
      <div className="relative">
        <Search className="absolute left-4 top-1/2 h-4 w-4 -translate-y-1/2 text-[color:var(--ink-900)]/40" />
        <input
          type="text"
          placeholder="Search help articles..."
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          className="h-12 w-full rounded-md border border-[color:var(--ink-900)]/10 bg-white pl-11 pr-4 text-sm text-[color:var(--ink-900)] placeholder:text-[color:var(--ink-900)]/40 focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
          aria-label="Search help articles"
        />
      </div>

      {showResults && (
        <div className="mt-2 rounded-md border border-[color:var(--ink-900)]/10 bg-white shadow-sm">
          {results.length === 0 ? (
            <p className="px-4 py-6 text-center text-sm text-[color:var(--ink-900)]/40">
              No articles found for &ldquo;{query}&rdquo;
            </p>
          ) : (
            <div className="divide-y divide-[color:var(--ink-900)]/[0.06]">
              {results.map((article) => (
                <Link
                  key={article.slug}
                  href={`/support/help/${article.slug}`}
                  className="flex flex-col gap-1 px-4 py-3 transition-colors hover:bg-[color:var(--ink-900)]/[0.02]"
                >
                  <span className="text-sm font-medium text-[color:var(--ink-900)]">
                    {article.title}
                  </span>
                  <span className="text-xs text-[color:var(--ink-900)]/40">
                    {article.category}
                  </span>
                </Link>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
```

**Verification:**
- [ ] Page renders at `/support/help`
- [ ] Category grid shows 5 categories with article counts
- [ ] Search filters articles by title and excerpt on keystroke
- [ ] Article list shows all 14 articles sorted by order
- [ ] Clicking an article navigates to `/support/help/[slug]`

---

### Task 5: Article page with markdown renderer

**File: `apps/admin/app/support/help/[slug]/page.tsx`**

```tsx
import { AdminShell } from "@/components/shell/AdminShell";
import { getServerSessionContext } from "@/lib/auth/serverSession";
import { getArticleBySlug, getAllArticles } from "@/lib/help";
import { HelpArticleRenderer } from "@/components/support/HelpArticleRenderer";
import { HelpArticleFeedback } from "@/components/support/HelpArticleFeedback";
import { notFound } from "next/navigation";
import Link from "next/link";
import { ArrowLeft } from "lucide-react";

interface ArticlePageProps {
  params: Promise<{ slug: string }>;
}

export default async function ArticlePage({ params }: ArticlePageProps) {
  const { slug } = await params;
  const session = await getServerSessionContext();
  const { tenantName, email } = session;

  const article = getArticleBySlug(slug);
  if (!article) {
    notFound();
  }

  // Related articles: same category, excluding current article.
  const allArticles = getAllArticles();
  const related = allArticles
    .filter((a) => a.category === article.category && a.slug !== slug)
    .slice(0, 3);

  return (
    <AdminShell tenantName={tenantName} userEmail={email}>
      <main className="mx-auto max-w-2xl px-8 py-8">
        <div className="flex flex-col gap-8">
          {/* Breadcrumb */}
          <nav aria-label="Breadcrumb" className="flex items-center gap-2 text-xs text-[color:var(--ink-900)]/40">
            <Link
              href="/support/help"
              className="transition-colors hover:text-[color:var(--ink-900)]"
            >
              Help Center
            </Link>
            <span aria-hidden="true">/</span>
            <span>{article.category}</span>
            <span aria-hidden="true">/</span>
            <span className="text-[color:var(--ink-900)]/60">{article.title}</span>
          </nav>

          {/* Article title */}
          <h1 className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-4xl leading-tight text-[color:var(--ink-900)]">
            {article.title}
          </h1>

          {/* Rendered markdown */}
          <HelpArticleRenderer content={article.content} />

          {/* Feedback */}
          <HelpArticleFeedback />

          {/* Related articles */}
          {related.length > 0 && (
            <div className="border-t border-[color:var(--ink-900)]/[0.06] pt-8">
              <h2 className="text-sm font-semibold uppercase tracking-[0.08em] text-[color:var(--ink-900)]/40">
                Related Articles
              </h2>
              <div className="mt-4 flex flex-col gap-2">
                {related.map((r) => (
                  <Link
                    key={r.slug}
                    href={`/support/help/${r.slug}`}
                    className="text-sm text-[color:var(--moss-700)] transition-opacity hover:opacity-80"
                  >
                    {r.title}
                  </Link>
                ))}
              </div>
            </div>
          )}

          {/* Back link */}
          <Link
            href="/support/help"
            className="inline-flex items-center gap-2 text-sm text-[color:var(--ink-900)]/60 transition-colors hover:text-[color:var(--ink-900)]"
          >
            <ArrowLeft className="h-4 w-4" />
            Back to Help Center
          </Link>
        </div>
      </main>
    </AdminShell>
  );
}
```

**File: `apps/admin/components/support/HelpArticleRenderer.tsx`**

```tsx
import { marked } from "marked";
import DOMPurify from "dompurify";
import { JSDOM } from "jsdom";

interface HelpArticleRendererProps {
  content: string;
}

// Server-side DOMPurify needs a jsdom window.
const window = new JSDOM("").window;
const purify = DOMPurify(window as unknown as Window);

/**
 * Renders markdown to sanitized HTML. This is a server component —
 * the markdown is rendered at request time, not on the client.
 *
 * Styling uses Tailwind prose classes adapted for Paper/Ink/Moss.
 */
export function HelpArticleRenderer({ content }: HelpArticleRendererProps) {
  const rawHtml = marked.parse(content, { async: false }) as string;
  const safeHtml = purify.sanitize(rawHtml);

  return (
    <div
      className="prose prose-sm max-w-none
        prose-headings:font-[family-name:var(--font-serif,'Source_Serif_4',serif)]
        prose-headings:text-[color:var(--ink-900)]
        prose-headings:tracking-tight
        prose-h2:mt-8 prose-h2:text-xl
        prose-h3:mt-6 prose-h3:text-lg
        prose-p:text-[color:var(--ink-900)]/80
        prose-p:leading-relaxed
        prose-a:text-[color:var(--moss-700)]
        prose-a:no-underline prose-a:hover:underline
        prose-strong:text-[color:var(--ink-900)]
        prose-code:rounded prose-code:bg-[color:var(--ink-900)]/[0.04]
        prose-code:px-1.5 prose-code:py-0.5
        prose-code:text-[color:var(--ink-900)]/80
        prose-code:before:content-none prose-code:after:content-none
        prose-ul:text-[color:var(--ink-900)]/80
        prose-ol:text-[color:var(--ink-900)]/80
        prose-li:marker:text-[color:var(--ink-900)]/30
        prose-table:text-sm
        prose-th:text-left prose-th:font-semibold
        prose-th:text-[color:var(--ink-900)]/60
        prose-td:text-[color:var(--ink-900)]/80
        prose-hr:border-[color:var(--ink-900)]/[0.06]"
      dangerouslySetInnerHTML={{ __html: safeHtml }}
    />
  );
}
```

Also install `jsdom` for server-side DOMPurify:

```bash
cd apps/admin
npm install jsdom
npm install -D @types/jsdom
```

**File: `apps/admin/components/support/HelpArticleFeedback.tsx`**

```tsx
"use client";

import { useState } from "react";
import { ThumbsUp, ThumbsDown } from "lucide-react";

export function HelpArticleFeedback() {
  const [feedback, setFeedback] = useState<"yes" | "no" | null>(null);

  if (feedback) {
    return (
      <div className="rounded-md border border-[color:var(--ink-900)]/[0.06] bg-[color:var(--ink-900)]/[0.02] px-4 py-3">
        <p className="text-sm text-[color:var(--ink-900)]/60">
          Thank you for your feedback.
        </p>
      </div>
    );
  }

  return (
    <div className="rounded-md border border-[color:var(--ink-900)]/[0.06] bg-[color:var(--ink-900)]/[0.02] px-4 py-4">
      <p className="text-sm font-medium text-[color:var(--ink-900)]">
        Was this article helpful?
      </p>
      <div className="mt-3 flex items-center gap-3">
        <button
          type="button"
          onClick={() => setFeedback("yes")}
          className="inline-flex h-9 items-center gap-2 rounded-md border border-[color:var(--ink-900)]/10 px-4 text-sm text-[color:var(--ink-900)]/60 transition-colors hover:border-[color:var(--moss-700)]/30 hover:text-[color:var(--moss-700)]"
        >
          <ThumbsUp className="h-3.5 w-3.5" aria-hidden="true" />
          Yes
        </button>
        <button
          type="button"
          onClick={() => setFeedback("no")}
          className="inline-flex h-9 items-center gap-2 rounded-md border border-[color:var(--ink-900)]/10 px-4 text-sm text-[color:var(--ink-900)]/60 transition-colors hover:border-[color:var(--ink-900)]/20 hover:text-[color:var(--ink-900)]"
        >
          <ThumbsDown className="h-3.5 w-3.5" aria-hidden="true" />
          No
        </button>
      </div>
    </div>
  );
}
```

**Verification:**
- [ ] Page renders at `/support/help/payments` (and all other slugs)
- [ ] Markdown renders correctly with heading hierarchy, lists, tables, links, code
- [ ] Breadcrumb shows Help Center > Category > Article
- [ ] Related articles show from same category
- [ ] Feedback buttons toggle to "Thank you" message
- [ ] 404 page renders for `/support/help/nonexistent`

---

### Task 6: HelpLink component

Create the reusable contextual link component.

**File: `apps/admin/components/support/HelpLink.tsx`**

```tsx
import Link from "next/link";

interface HelpLinkProps {
  /** The help article slug (e.g., "payments", "shipping"). */
  slug: string;
  /** Optional custom label. Defaults to "Learn more". */
  label?: string;
}

/**
 * HelpLink renders a small contextual link to a help article.
 * Used in page headers across settings and feature pages.
 *
 * Usage: <HelpLink slug="payments" />
 * Renders: <a href="/support/help/payments">Learn more →</a>
 */
export function HelpLink({ slug, label = "Learn more" }: HelpLinkProps) {
  return (
    <Link
      href={`/support/help/${slug}`}
      className="text-xs text-[color:var(--moss-700)] opacity-60 transition-opacity hover:opacity-100"
    >
      {label} &rarr;
    </Link>
  );
}
```

**Verification:**
- [ ] `<HelpLink slug="payments" />` renders `<a href="/support/help/payments">Learn more →</a>`
- [ ] Styled with moss-700 color at 60% opacity
- [ ] Hover increases opacity to 100%

---

### Task 7: Add HelpLink to settings pages

Add contextual `HelpLink` components to existing settings and feature page headers. For each page, import the component and add it near the page title.

**Pattern for each page:**

```tsx
import { HelpLink } from "@/components/support/HelpLink";

// In the page header, next to or below the title:
<div className="flex items-center gap-3">
  <h1 className="...">Payments</h1>
  <HelpLink slug="payments" />
</div>
```

**Pages to update:**

1. **`apps/admin/app/settings/payments/page.tsx`** — add `<HelpLink slug="payments" />`
2. **`apps/admin/app/settings/shipping/page.tsx`** — add `<HelpLink slug="shipping" />`
3. **`apps/admin/app/settings/tax/page.tsx`** — add `<HelpLink slug="tax" />`
4. **`apps/admin/app/products/page.tsx`** — add `<HelpLink slug="products" />`
5. **`apps/admin/app/orders/page.tsx`** — add `<HelpLink slug="orders" />` (if the page exists)
6. **`apps/admin/app/settings/team/page.tsx`** — add `<HelpLink slug="team" />` (if the page exists)
7. **`apps/admin/app/settings/storefront/page.tsx`** — add `<HelpLink slug="storefront" />` (if the page exists)

**Important:** Only add HelpLink to pages that already exist. Do not create placeholder pages just to add a HelpLink. Check each file exists before modifying.

**Example modification for `apps/admin/app/settings/payments/page.tsx`:**

Find the page title/header section and add the HelpLink next to it:

```tsx
// Before:
<h1 className="...">Payments</h1>

// After:
<div className="flex items-center gap-3">
  <h1 className="...">Payments</h1>
  <HelpLink slug="payments" />
</div>
```

**Verification:**
- [ ] Each modified settings page shows a "Learn more →" link
- [ ] Clicking the link navigates to the correct help article
- [ ] The link is visually subtle (small text, reduced opacity)
- [ ] No pages were accidentally created (only existing pages modified)

---

## Summary

| Task | Scope | Files |
|------|-------|-------|
| 1 | Dependencies | package.json (marked, dompurify, gray-matter, jsdom) |
| 2 | Content | 14 markdown files in content/help/ |
| 3 | Article loader | lib/help.ts |
| 4 | Landing page | page.tsx, HelpCategoryGrid.tsx, HelpArticleList.tsx, HelpSearch.tsx |
| 5 | Article page | [slug]/page.tsx, HelpArticleRenderer.tsx, HelpArticleFeedback.tsx |
| 6 | HelpLink | HelpLink.tsx |
| 7 | Contextual links | 5-7 existing page modifications |

**Total new files:** ~22 (14 markdown + 8 TypeScript) | **Modified files:** ~6-8
