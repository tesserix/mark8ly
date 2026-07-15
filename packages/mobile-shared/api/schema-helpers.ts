import { z } from "zod";

/**
 * Money crosses the wire in TWO real shapes: a JSON number (dashboard.go
 * uses float64) and a quoted string (orders/products marshal
 * shopspring/decimal, which quotes unless MarshalJSONWithoutQuotes is set —
 * repo-wide grep confirms it never is).
 *
 * NOT z.coerce.number(): coerce turns null -> 0, "" -> 0 and true -> 1.
 * A silently wrong price is worse than a loud failure. The .min(1) rejects
 * empty/whitespace strings (a bare union would let "" through as 0) and the
 * .pipe(finite) rejects "abc" (a bare union would yield NaN).
 */
export const money = z
  .union([z.number(), z.string().trim().min(1)])
  .transform(Number)
  .pipe(z.number().finite());

/** Every paginated list endpoint returns this meta block. */
export const pageMeta = z.object({
  page: z.number(),
  page_size: z.number(),
  total: z.number(),
  total_pages: z.number(),
});

/** The standard list envelope: `{data, meta}`. */
export const paginated = <T extends z.ZodTypeAny>(item: T) =>
  z.object({ data: z.array(item), meta: pageMeta });

/**
 * `/stores` returns `{data}` with NO meta (stores.go:74) — kept separate from
 * `paginated` deliberately, so we never invent a meta the endpoint doesn't send.
 */
export const dataOnly = <T extends z.ZodTypeAny>(item: T) =>
  z.object({ data: z.array(item) });
