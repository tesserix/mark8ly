/**
 * Returns `href` if it uses an allowed scheme (http://, https://, or a
 * site-relative path starting with /), otherwise returns a safe fallback
 * ("#"). Mirrors the backend IsSafeURL check — acts as a second line of
 * defense so a stored XSS bypass can't leak out of any single layer.
 */
export function safeUrl(href: string | null | undefined): string {
  if (!href) return "#";
  const trimmed = href.trim();
  if (trimmed.startsWith("http://") || trimmed.startsWith("https://") || trimmed.startsWith("/")) {
    return trimmed;
  }
  return "#";
}
