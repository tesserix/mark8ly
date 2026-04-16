import { headers } from "next/headers";

import { resolveStoreSlug } from "@/lib/slug";
import { fetchBranding } from "@/lib/api/marketplace-api";

/**
 * /ai.txt — opt-out signal for AI training crawlers.
 *
 * There's no single official standard, so we emit a pragmatic text
 * summary of the AI policy. Crawlers that respect it (some do) will
 * self-limit; those that don't are also blocked via robots.txt rules.
 */
export async function GET() {
  const h = await headers();
  const host = h.get("host");
  const slug = await resolveStoreSlug(host);

  let policy: "allow" | "deny" | "training-only-denied" = "allow";
  if (slug) {
    const branding = await fetchBranding(slug).catch(() => null);
    policy = branding?.branding?.seo_ai_policy ?? "allow";
  }

  const lines: string[] = [
    "# ai.txt — AI training and crawler policy",
    `# Host: ${host ?? "unknown"}`,
    "",
  ];

  if (policy === "allow") {
    lines.push(
      "Policy: allow",
      "AI crawlers may index this site and use it for training.",
    );
  } else if (policy === "training-only-denied") {
    lines.push(
      "Policy: no-training",
      "AI assistants may cite and link to this site.",
      "Use of this site's content for training machine-learning models is NOT authorized.",
      "User-Agent: *",
      "Disallow-Training: /",
    );
  } else {
    lines.push(
      "Policy: deny",
      "AI crawlers are not authorized to index this site.",
      "Use of this site's content for training machine-learning models is NOT authorized.",
      "User-Agent: *",
      "Disallow: /",
      "Disallow-Training: /",
    );
  }

  return new Response(lines.join("\n") + "\n", {
    headers: {
      "Content-Type": "text/plain; charset=utf-8",
      "Cache-Control": "public, max-age=300",
    },
  });
}
