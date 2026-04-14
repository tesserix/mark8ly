import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

export type MarkdownProps = {
  children: string;
  className?: string;
};

/**
 * Renders user-authored markdown with GFM (tables, strikethrough, task lists).
 *
 * Security: `skipHtml` is the XSS guard. Merchant-authored markdown can include
 * raw <script> or <iframe> tags; `skipHtml` strips them before render. Do not
 * remove this prop without adding a DOMPurify pass first.
 */
export function Markdown({ children, className }: MarkdownProps) {
  return (
    <div className={className}>
      <ReactMarkdown remarkPlugins={[remarkGfm]} skipHtml>
        {children}
      </ReactMarkdown>
    </div>
  );
}
