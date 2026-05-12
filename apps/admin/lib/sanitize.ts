import DOMPurify from "isomorphic-dompurify";

// Tags permitted in merchant-authored rich text (campaign bodies, help
// articles). Deliberately excludes <script>, <iframe>, <object>, <embed>,
// <form>, <style>, and any tag that can fire JS via attributes.
const ALLOWED_TAGS = [
  "a",
  "b",
  "blockquote",
  "br",
  "code",
  "em",
  "h1",
  "h2",
  "h3",
  "h4",
  "h5",
  "h6",
  "hr",
  "i",
  "img",
  "li",
  "ol",
  "p",
  "pre",
  "s",
  "span",
  "strong",
  "sub",
  "sup",
  "table",
  "tbody",
  "td",
  "th",
  "thead",
  "tr",
  "u",
  "ul",
];

const ALLOWED_ATTR = [
  "href",
  "src",
  "alt",
  "title",
  "id",
  "class",
  "target",
  "rel",
  "width",
  "height",
];

// sanitizeRichHtml strips every tag/attribute outside the allowlist and
// blocks javascript:/data: URIs in href/src. DOMPurify also rejects
// mutation-XSS payloads (clobbering, namespace confusion) that hand-
// rolled regex sanitizers historically miss.
export function sanitizeRichHtml(dirty: string): string {
  return DOMPurify.sanitize(dirty, {
    ALLOWED_TAGS,
    ALLOWED_ATTR,
    ALLOW_DATA_ATTR: false,
    FORBID_TAGS: ["style", "script", "iframe", "object", "embed", "form"],
    FORBID_ATTR: ["onerror", "onload", "onclick", "onmouseover", "onfocus"],
  });
}
