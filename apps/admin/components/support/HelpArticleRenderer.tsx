import { marked } from "marked";

interface HelpArticleRendererProps {
  content: string;
}

export function HelpArticleRenderer({ content }: HelpArticleRendererProps) {
  const html = marked.parse(content, {
    async: false,
    gfm: true,
    breaks: false,
  }) as string;

  return (
    <div
      className="help-article"
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}
