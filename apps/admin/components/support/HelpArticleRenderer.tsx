import { marked } from "marked";

interface HelpArticleRendererProps {
  content: string;
}

export function HelpArticleRenderer({ content }: HelpArticleRendererProps) {
  const html = marked.parse(content, { async: false }) as string;

  return (
    <div
      className="prose prose-sm max-w-none text-foreground prose-headings:font-serif prose-headings:font-medium prose-headings:text-foreground prose-h2:mt-10 prose-h2:text-2xl prose-h3:text-lg prose-p:leading-7 prose-p:text-foreground-secondary prose-a:text-[color:var(--moss-700)] prose-a:no-underline hover:prose-a:underline prose-strong:text-foreground prose-li:text-foreground-secondary prose-code:rounded prose-code:bg-paper-100 prose-code:px-1 prose-code:py-0.5 prose-code:text-foreground"
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}
