# URI decoder compatibility

The fixed `decode-uri-component@0.5.1` release is ESM-only. The existing
`query-string@7` consumers require a callable CommonJS export. This bridge
preserves that API and delegates all parsing to the patched upstream package
through the `safe-decode-uri-component` npm alias.

The root override applies it to existing navigation consumers. Remove the
bridge when all consumers support the upstream ESM API. Jest transforms the
safe alias so the same bridge works under its CommonJS test runtime.
