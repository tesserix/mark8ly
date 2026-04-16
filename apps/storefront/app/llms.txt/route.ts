import { headers } from "next/headers";

import { resolveStoreSlug } from "@/lib/slug";
import { fetchBranding } from "@/lib/api/marketplace-api";
import { fetchStoreBySlug } from "@/lib/api/platform-api";

/**
 * /llms.txt — emergent standard (llmstxt.org) for LLM agents.
 *
 * Serves the merchant's custom llms.txt if set; otherwise auto-generates
 * a minimal summary from store name + branding description.
 *
 * Content-Type: text/plain; charset=utf-8
 */
export async function GET() {
  const h = await headers();
  const host = h.get("host");
  const slug = await resolveStoreSlug(host);

  if (!slug) {
    return new Response("# Not found\n", {
      status: 404,
      headers: { "Content-Type": "text/plain; charset=utf-8" },
    });
  }

  const [branding, store] = await Promise.all([
    fetchBranding(slug).catch(() => null),
    fetchStoreBySlug(slug).catch(() => null),
  ]);

  const custom = branding?.branding?.seo_llms_txt?.trim();
  if (custom) {
    return new Response(custom + "\n", {
      headers: {
        "Content-Type": "text/plain; charset=utf-8",
        "Cache-Control": "public, max-age=300",
      },
    });
  }

  const name = store?.name ?? "Store";
  const description =
    branding?.branding?.seo_default_description?.trim() ??
    branding?.branding?.footer_tagline?.trim() ??
    `${name} on Mark8ly.`;

  const body = [
    `# ${name}`,
    "",
    `> ${description}`,
    "",
    "## Shop",
    `- [All products](/products): Browse the full catalog`,
    `- [Collections](/categories): Curated collections`,
    "",
    "## About",
    `- [Store home](/): Landing page with featured products and story`,
  ].join("\n");

  return new Response(body + "\n", {
    headers: {
      "Content-Type": "text/plain; charset=utf-8",
      "Cache-Control": "public, max-age=300",
    },
  });
}
