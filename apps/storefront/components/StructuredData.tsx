/**
 * Emits a schema.org JSON-LD <script>. The payload is serialized here so
 * every caller gets the same XSS-safe escaping: `<` is replaced with its
 * unicode escape so merchant-controlled values (product titles, store
 * names, descriptions) can never inject a literal `</script>` and break
 * out of the tag. JSON.stringify already escapes quotes; `<` is the only
 * character that matters for script-context breakout.
 */
export function StructuredData({ data }: { data: Record<string, unknown> }) {
  const json = JSON.stringify(data).replace(/</g, "\\u003c");
  return (
    <script
      type="application/ld+json"
      dangerouslySetInnerHTML={{ __html: json }}
    />
  );
}
