import fs from "node:fs";
import path from "node:path";

export interface HelpArticleMeta {
  slug: string;
  title: string;
  category: string;
  order: number;
  excerpt: string;
}

export interface HelpArticle {
  slug: string;
  title: string;
  category: string;
  content: string;
}

export interface HelpHeading {
  slug: string;
  text: string;
  level: 2 | 3;
}

const HELP_DIR = path.join(process.cwd(), "content", "help");

/** Allowlisted slugs — only these can be loaded to prevent path traversal. */
const VALID_SLUGS = [
  "getting-started",
  "products",
  "orders",
  "payments",
  "shipping",
  "tax",
  "customers",
  "reviews",
  "marketing",
  "domains",
  "team",
  "subscription",
  "storefront",
  "troubleshooting",
  "customer-support-kb",
] as const;

interface Frontmatter {
  title: string;
  category: string;
  order: number;
}

function parseFrontmatter(raw: string): {
  meta: Frontmatter;
  content: string;
} {
  const lines = raw.split("\n");
  if (lines[0]?.trim() !== "---") {
    return {
      meta: { title: "", category: "", order: 999 },
      content: raw,
    };
  }

  const endIdx = lines.indexOf("---", 1);
  if (endIdx === -1) {
    return {
      meta: { title: "", category: "", order: 999 },
      content: raw,
    };
  }

  const frontLines = lines.slice(1, endIdx);
  const meta: Record<string, string> = {};
  for (const line of frontLines) {
    const colonIdx = line.indexOf(":");
    if (colonIdx === -1) continue;
    const key = line.slice(0, colonIdx).trim();
    const val = line.slice(colonIdx + 1).trim().replace(/^["']|["']$/g, "");
    meta[key] = val;
  }

  const content = lines.slice(endIdx + 1).join("\n").trim();

  return {
    meta: {
      title: meta.title ?? "",
      category: meta.category ?? "",
      order: Number(meta.order) || 999,
    },
    content,
  };
}

/**
 * Load all help articles sorted by order.
 */
export function getArticles(): HelpArticleMeta[] {
  const articles: HelpArticleMeta[] = [];

  for (const slug of VALID_SLUGS) {
    const filePath = path.join(HELP_DIR, `${slug}.md`);
    if (!fs.existsSync(filePath)) continue;

    const raw = fs.readFileSync(filePath, "utf-8");
    const { meta, content } = parseFrontmatter(raw);

    // Strip markdown headings for excerpt
    const plainText = content
      .replace(/^#+\s+.*/gm, "")
      .replace(/\n{2,}/g, " ")
      .trim();
    const excerpt = plainText.slice(0, 200);

    articles.push({
      slug,
      title: meta.title,
      category: meta.category,
      order: meta.order,
      excerpt,
    });
  }

  return articles.sort((a, b) => a.order - b.order);
}

/**
 * Load a single help article by slug.
 */
export function getArticle(slug: string): HelpArticle | null {
  // Validate against allowlist
  const safeSlug = path.basename(slug);
  if (!(VALID_SLUGS as readonly string[]).includes(safeSlug)) {
    return null;
  }

  const filePath = path.join(HELP_DIR, `${safeSlug}.md`);
  if (!fs.existsSync(filePath)) {
    return null;
  }

  const raw = fs.readFileSync(filePath, "utf-8");
  const { meta, content } = parseFrontmatter(raw);

  return {
    slug: safeSlug,
    title: meta.title,
    category: meta.category,
    content,
  };
}

/**
 * Extract h2/h3 headings from markdown content for the on-page TOC.
 * Skips lines inside fenced code blocks. Slug matches the renderer's id.
 */
export function getHeadings(content: string): HelpHeading[] {
  const lines = content.split("\n");
  const out: HelpHeading[] = [];
  let inFence = false;
  for (const raw of lines) {
    const line = raw.trimEnd();
    if (line.startsWith("```")) {
      inFence = !inFence;
      continue;
    }
    if (inFence) continue;
    const m = line.match(/^(#{2,3})\s+(.+?)\s*$/);
    if (!m || !m[1] || !m[2]) continue;
    const level: 2 | 3 = m[1].length === 2 ? 2 : 3;
    const text = m[2].replace(/[*_`]/g, "").trim();
    out.push({ slug: slugify(text), text, level });
  }
  return out;
}

function slugify(s: string): string {
  return s
    .toLowerCase()
    .replace(/[^a-z0-9\s-]/g, "")
    .trim()
    .replace(/\s+/g, "-");
}
