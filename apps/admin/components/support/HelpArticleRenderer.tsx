import { marked } from "marked";

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

  const html = raw.replace(
    /<(h[23])>([\s\S]*?)<\/h[23]>/g,
    (_match, tag: string, inner: string) => {
      const text = inner.replace(/<[^>]+>/g, "").trim();
      const id = slugify(text);
      return `<${tag} id="${id}">${inner}</${tag}>`;
    },
  );

  return (
    <div
      className="help-article"
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}
