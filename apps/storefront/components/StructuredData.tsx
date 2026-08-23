import { headers } from "next/headers";

/**
 * Emits a schema.org JSON-LD <script>. The payload is serialized here so
 * every caller gets the same XSS-safe escaping: `<` is replaced with its
 * unicode escape so merchant-controlled values (product titles, store
 * names, descriptions) can never inject a literal `</script>` and break
 * out of the tag. JSON.stringify already escapes quotes; `<` is the only
 * character that matters for script-context breakout.
 *
 * The nonce comes from middleware — script-src no longer allows inline
 * blocks without one, and browsers apply that check to JSON-LD too.
 */
export async function StructuredData({ data }: { data: Record<string, unknown> }) {
  const json = JSON.stringify(data).replace(/</g, "\\u003c");
  const nonce = (await headers()).get("x-nonce") ?? undefined;
  return (
    <script
      type="application/ld+json"
      nonce={nonce}
      dangerouslySetInnerHTML={{ __html: json }}
    />
  );
}
