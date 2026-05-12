import { marked } from "marked";
import { sanitizeRichHtml } from "@/lib/sanitize";

interface HelpArticleRendererProps {
  content: string;
}

function slugify(s: string): string {
  return s
    .toLowerCase()
    .replace(/[^a-z0-9\s-]/g, "")
    .trim()
    .replace(/\s+/g, "-");
}

export function HelpArticleRenderer({ content }: HelpArticleRendererProps) {
  const raw = marked.parse(content, {
    async: false,
    gfm: true,
    breaks: false,
  }) as string;

  const withAnchors = raw.replace(
    /<(h[23])>([\s\S]*?)<\/h[23]>/g,
    (_match, tag: string, inner: string) => {
      const text = inner.replace(/<[^>]+>/g, "").trim();
      const id = slugify(text);
      return `<${tag} id="${id}">${inner}</${tag}>`;
    },
  );

  // marked passes raw HTML in the markdown source through untouched —
  // sanitize before injection so a help article containing <script> or
  // <img onerror=…> can't fire in the admin DOM.
  const html = sanitizeRichHtml(withAnchors);

  return (
    <div
      className="help-article"
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}
