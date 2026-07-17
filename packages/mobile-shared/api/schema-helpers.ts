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

/**
 * Non-money decimals (variant dimensions) have the SAME wire reality as money:
 * shopspring/decimal quotes them, so they arrive as "30.5" not 30. Aliased
 * rather than duplicated — the parsing rules are identical, only the meaning
 * differs, and `money.length_cm` would read as nonsense.
 */
export const decimalNumber = money;

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

/**
 * The odd envelope out: `{<key>: [...], page, per_page, total}`.
 *
 * Only /notifications uses it (notifications.go:85) — every other list
 * endpoint returns `{data, meta}`. Verified live 2026-07-16:
 * `{"notifications":[],"page":1,"per_page":20,"total":0}`.
 *
 * Named `legacy` because this SHOULD be normalised to `paginated` server-side
 * one day; until then the app must not pretend the shape is something it is
 * not. Note `per_page` here vs `page_size` in pageMeta — also inconsistent,
 * also real.
 */
export const legacyPaged = <K extends string, T extends z.ZodTypeAny>(key: K, item: T) =>
  z.object({
    [key]: z.array(item),
    page: z.number(),
    per_page: z.number(),
    total: z.number(),
  } as { [P in K]: z.ZodArray<T> } & {
    page: z.ZodNumber;
    per_page: z.ZodNumber;
    total: z.ZodNumber;
  });

/**
 * The marketing handlers wrap EVERYTHING in `{data: ...}` (unlike orders/reviews
 * which return bare objects from get/mutations). A single resource comes back as
 * `{data: <obj>}`; the api layer parses this and returns `.data`.
 */
export const enveloped = <T extends z.ZodTypeAny>(item: T) => z.object({ data: item });

/** `{data: <obj> | null}` — loyalty GetProgram returns `{data: null}` when the
 *  store has never configured a program. */
export const envelopedNullable = <T extends z.ZodTypeAny>(item: T) =>
  z.object({ data: item.nullable() });

/** Coupons list envelope: `{data: [...], total, page}` (coupons.go:73) — NOT the
 *  standard `{data, meta}`; there is no page_size/total_pages here. */
export const envelopedListTotalPage = <T extends z.ZodTypeAny>(item: T) =>
  z.object({ data: z.array(item), total: z.number(), page: z.number() });

/** Loyalty list envelope: `{data: [...], meta: {total, page, limit}}` (loyalty.go)
 *  — a THIRD meta shape (total/page/limit, not page/page_size/total/total_pages). */
export const loyaltyMeta = z.object({ total: z.number(), page: z.number(), limit: z.number() });
export const loyaltyPaged = <T extends z.ZodTypeAny>(item: T) =>
  z.object({ data: z.array(item), meta: loyaltyMeta });
