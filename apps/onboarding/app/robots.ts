import type { MetadataRoute } from "next";

/**
 * robots.txt — Next 16 file convention. Generated at build
 * from this route, served at /robots.txt.
 *
 * ── FUNNEL EXCLUSIONS ────────────────────────────────────────────
 * The onboarding funnel and /welcome are conversion surfaces, not
 * marketing ones — we do not want them in search results or in an
 * LLM's answer about what Mark8ly is.
 *
 * `/onboarding` is written WITHOUT a trailing slash on purpose. The
 * live route is `/onboarding`; a rule of `/onboarding/` does not
 * match it under standard prefix matching, so the page this rule
 * exists to exclude was in fact crawlable (tesserix/mark8ly#603).
 * Without the slash the prefix covers both the route itself and
 * everything beneath it.
 *
 * ── AI CRAWLERS: RETRIEVAL YES, TRAINING NO ──────────────────────
 * We want to be CITED, not INGESTED. A new domain cannot outrank
 * the affiliate listicles that own "shopify alternative" inside a
 * year, so being quotable in an AI answer is a realistic route to
 * visibility in a way that ranking for the head term is not. Being
 * absorbed into a training corpus buys us nothing in return.
 *
 * So the two classes are declared separately rather than left to a
 * wildcard, because "allow everything" and "allow the ones that
 * cite us" look identical in a permissive robots.txt and are very
 * different intentions. Anyone reading this file should be able to
 * see which one we chose.
 *
 * Note that a bot obeys ONLY its own most-specific group — it does
 * not inherit from `*`. That is why the funnel disallows are
 * repeated for the retrieval crawlers: omitting them would quietly
 * grant those bots access to the very paths the wildcard blocks.
 *
 * Two of the "training" entries are not crawlers at all.
 * `Google-Extended` and `Applebot-Extended` are control tokens that
 * only ever appear in robots.txt — no request is ever made under
 * those names, so they cannot be enforced at the edge and belong
 * here or nowhere. `Google-Extended` governs Gemini training and
 * grounding; it does NOT govern AI Overviews, which are built from
 * the Search index via Googlebot. Blocking it costs us no AI
 * Overview presence.
 *
 * ── THIS FILE IS A REQUEST, NOT A CONTROL ────────────────────────
 * robots.txt is advisory. The enforcement lives at the Cloudflare
 * edge, and the two must be kept in agreement — mark8ly#595 exists
 * because they were not: this file said `Allow: /` to everyone
 * while Cloudflare returned 403 to every AI user agent, retrieval
 * crawlers included. If you change the policy here, change it there
 * too, and verify with:
 *
 *   curl -s -o /dev/null -w "%{http_code}\n" \
 *     -A "OAI-SearchBot/1.0" https://mark8ly.com/     # expect 200
 *   curl -s -o /dev/null -w "%{http_code}\n" \
 *     -A "GPTBot/1.0" https://mark8ly.com/            # expect 403
 */

/** Paths that are conversion surfaces, not marketing ones. */
const FUNNEL_DISALLOW = ["/onboarding", "/welcome", "/api/"];

/**
 * Crawlers that fetch a page to answer a question and cite the
 * source. These are the ones the growth plan depends on.
 */
const RETRIEVAL_CRAWLERS = [
  "OAI-SearchBot",
  "ChatGPT-User",
  "PerplexityBot",
  "Perplexity-User",
  "Claude-User",
  "Claude-SearchBot",
];

/**
 * Crawlers that collect pages into training corpora, plus the two
 * robots-only control tokens that opt us out of model training at
 * Google and Apple.
 */
const TRAINING_CRAWLERS = [
  "GPTBot",
  "ClaudeBot",
  "anthropic-ai",
  "CCBot",
  "Bytespider",
  "meta-externalagent",
  "Google-Extended",
  "Applebot-Extended",
];

export default function robots(): MetadataRoute.Robots {
  return {
    rules: [
      {
        userAgent: "*",
        allow: "/",
        disallow: FUNNEL_DISALLOW,
      },
      {
        userAgent: RETRIEVAL_CRAWLERS,
        allow: "/",
        disallow: FUNNEL_DISALLOW,
      },
      {
        userAgent: TRAINING_CRAWLERS,
        disallow: "/",
      },
    ],
    sitemap: "https://mark8ly.com/sitemap.xml",
    host: "https://mark8ly.com",
  };
}
