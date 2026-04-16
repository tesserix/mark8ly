import type { MetadataRoute } from "next";
import { headers } from "next/headers";

import { resolveStoreSlug } from "@/lib/slug";
import { canonicalUrl } from "@/lib/seo";
import { fetchBranding } from "@/lib/api/marketplace-api";

/**
 * Per-tenant robots.txt.
 *
 * Reads the merchant's AI policy from branding and applies the right
 * disallow rules. `/api` is always blocked; generic crawlers are
 * otherwise allowed. AI-specific bots get targeted rules based on
 * policy:
 *   - allow:                  no special rules
 *   - training-only-denied:   AI bots crawl (for citations) but
 *                              Google-Extended / training use is blocked
 *   - deny:                   every known AI crawler is disallowed
 */
export default async function robots(): Promise<MetadataRoute.Robots> {
  const h = await headers();
  const host = h.get("host");
  const slug = await resolveStoreSlug(host);
  const base = slug ? canonicalUrl(slug) : `https://${host ?? "mark8ly.com"}`;

  let aiPolicy: "allow" | "deny" | "training-only-denied" = "allow";
  if (slug) {
    const branding = await fetchBranding(slug).catch(() => null);
    aiPolicy = branding?.branding?.seo_ai_policy ?? "allow";
  }

  const rules: MetadataRoute.Robots["rules"] = [
    {
      userAgent: "*",
      allow: "/",
      disallow: ["/api/", "/_next/"],
    },
  ];

  if (aiPolicy === "deny") {
    for (const bot of AI_CRAWLERS) {
      rules.push({ userAgent: bot, disallow: "/" });
    }
  } else if (aiPolicy === "training-only-denied") {
    for (const bot of AI_TRAINING_CRAWLERS) {
      rules.push({ userAgent: bot, disallow: "/" });
    }
  }

  return {
    rules,
    sitemap: `${base}/sitemap.xml`,
    host: base,
  };
}

// AI crawlers that both index and potentially train. Blocking these
// removes the store from AI assistant responses AND training data.
const AI_CRAWLERS = [
  "GPTBot",
  "ChatGPT-User",
  "OAI-SearchBot",
  "ClaudeBot",
  "Claude-Web",
  "anthropic-ai",
  "CCBot",
  "Google-Extended",
  "PerplexityBot",
  "Perplexity-User",
  "ByteSpider",
  "FacebookBot",
  "Meta-ExternalAgent",
  "Applebot-Extended",
  "Bytespider",
  "Amazonbot",
  "YouBot",
  "cohere-ai",
  "DuckAssistBot",
];

// Crawlers specifically known to harvest for model training.
// Blocking these keeps the store visible in AI assistants (citations,
// live-lookup tools) while refusing training use.
const AI_TRAINING_CRAWLERS = [
  "GPTBot",
  "CCBot",
  "Google-Extended",
  "anthropic-ai",
  "Meta-ExternalAgent",
  "Applebot-Extended",
  "ByteSpider",
  "Bytespider",
  "Amazonbot",
  "cohere-ai",
];
