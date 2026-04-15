"use client";

import { useEffect, useRef, useState } from "react";
import { Search, X } from "lucide-react";

import { FieldLabel } from "./BrandingSettingsClient";

const DEBOUNCE_MS = 250;
const MAX_RESULTS = 10;

interface ProductOption {
  slug: string;
  title: string;
  thumbnailUrl: string | null;
}

interface Props {
  storeId: string;
  value: string[];
  onChange: (next: string[]) => void;
  max: number;
  editable: boolean;
  label?: string;
  onMissingChange?: (missingSlugs: string[]) => void;
}

export function ProductSlugPicker({
  storeId,
  value,
  onChange,
  max,
  editable,
  label = "Hand-picked products",
  onMissingChange,
}: Props) {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<ProductOption[]>([]);
  const [loading, setLoading] = useState(false);
  const [resolvedLabels, setResolvedLabels] = useState<Record<string, ProductOption>>({});
  // Slugs that have been checked and returned no matching product.
  const [missingSlugs, setMissingSlugs] = useState<Set<string>>(new Set());
  const abortRef = useRef<AbortController | null>(null);

  // Resolve labels for currently-selected slugs so the chips show
  // titles instead of raw slugs. Fires when the selection changes.
  // Slugs that come back without a match are flagged as missing so
  // the form can show a warning badge — the merchant might have
  // renamed or deleted the product since they picked it.
  useEffect(() => {
    if (value.length === 0) {
      setMissingSlugs(new Set());
      return;
    }
    const unchecked = value.filter(
      (s) => !resolvedLabels[s] && !missingSlugs.has(s),
    );
    if (unchecked.length === 0) return;
    fetchProductsBySlugs(storeId, unchecked)
      .then((items) => {
        setResolvedLabels((prev) => {
          const next = { ...prev };
          for (const item of items) next[item.slug] = item;
          return next;
        });
        const returned = new Set(items.map((i) => i.slug));
        const freshlyMissing = unchecked.filter((s) => !returned.has(s));
        if (freshlyMissing.length > 0) {
          setMissingSlugs((prev) => {
            const next = new Set(prev);
            for (const s of freshlyMissing) next.add(s);
            return next;
          });
        }
      })
      .catch(() => {
        /* chips fall back to slug-only display; no missing flag on error */
      });
  }, [storeId, value, resolvedLabels, missingSlugs]);

  // Publish the current missing-slug set to the parent whenever it
  // changes, so the containing form can render a warning banner.
  useEffect(() => {
    if (!onMissingChange) return;
    const stillPicked = Array.from(missingSlugs).filter((s) => value.includes(s));
    onMissingChange(stillPicked);
  }, [missingSlugs, value, onMissingChange]);

  // Debounced search.
  useEffect(() => {
    const trimmed = query.trim();
    if (!trimmed) {
      setResults([]);
      return;
    }
    const handle = setTimeout(() => {
      abortRef.current?.abort();
      const ctrl = new AbortController();
      abortRef.current = ctrl;
      setLoading(true);
      fetchProducts(storeId, trimmed, ctrl.signal)
        .then((items) => {
          setResults(items);
        })
        .catch((err: unknown) => {
          if ((err as { name?: string } | null)?.name !== "AbortError") {
            setResults([]);
          }
        })
        .finally(() => setLoading(false));
    }, DEBOUNCE_MS);
    return () => clearTimeout(handle);
  }, [storeId, query]);

  const atMax = value.length >= max;
  const selectedSet = new Set(value);

  const add = (opt: ProductOption) => {
    if (atMax || selectedSet.has(opt.slug)) return;
    setResolvedLabels((prev) => ({ ...prev, [opt.slug]: opt }));
    onChange([...value, opt.slug]);
    setQuery("");
    setResults([]);
  };

  const remove = (slug: string) => onChange(value.filter((s) => s !== slug));

  return (
    <div className="space-y-2">
      <FieldLabel htmlFor="product_slug_picker_search">
        {label} ({value.length}/{max})
      </FieldLabel>

      {value.length > 0 ? (
        <>
          <ul className="flex flex-wrap gap-2">
            {value.map((slug) => {
              const resolved = resolvedLabels[slug];
              const isMissing = missingSlugs.has(slug) && !resolved;
              return (
                <li
                  key={slug}
                  className={
                    isMissing
                      ? "inline-flex items-center gap-2 rounded-md border border-danger bg-danger/5 px-2 py-1 text-xs text-danger"
                      : "inline-flex items-center gap-2 rounded-md border border-border bg-[color:var(--background-elevated)] px-2 py-1 text-xs text-foreground"
                  }
                >
                  {resolved?.thumbnailUrl ? (
                    // eslint-disable-next-line @next/next/no-img-element
                    <img
                      src={resolved.thumbnailUrl}
                      alt=""
                      className="h-6 w-6 rounded object-cover"
                    />
                  ) : null}
                  <span className="max-w-[12rem] truncate">
                    {resolved?.title ?? slug}
                  </span>
                  {isMissing ? (
                    <span className="rounded-full bg-danger/15 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide">
                      Missing
                    </span>
                  ) : null}
                  {editable ? (
                    <button
                      type="button"
                      onClick={() => remove(slug)}
                      className={
                        isMissing
                          ? "text-danger hover:opacity-80"
                          : "text-foreground-secondary hover:text-danger"
                      }
                      aria-label={`Remove ${resolved?.title ?? slug}`}
                    >
                      <X className="h-3 w-3" />
                    </button>
                  ) : null}
                </li>
              );
            })}
          </ul>
          {Array.from(missingSlugs).some((s) => value.includes(s)) ? (
            <p className="text-xs text-danger">
              Some picked products no longer exist — customers won&apos;t see them until you swap them out.
            </p>
          ) : null}
        </>
      ) : null}

      {editable && !atMax ? (
        <div className="relative">
          <div className="flex items-center gap-2 rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] px-3">
            <Search className="h-4 w-4 text-foreground-secondary" />
            <input
              id="product_slug_picker_search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search products by title…"
              className="h-10 flex-1 bg-transparent text-sm text-foreground placeholder:text-foreground-tertiary focus:outline-none"
            />
          </div>
          {query.trim() && (loading || results.length > 0) ? (
            <ul className="absolute z-10 mt-1 max-h-64 w-full overflow-auto rounded-[var(--radius)] border border-border bg-[color:var(--background-elevated)] shadow-lg">
              {loading ? (
                <li className="px-3 py-2 text-xs text-foreground-secondary">
                  Searching…
                </li>
              ) : null}
              {results.slice(0, MAX_RESULTS).map((r) => {
                const alreadySelected = selectedSet.has(r.slug);
                return (
                  <li key={r.slug}>
                    <button
                      type="button"
                      disabled={alreadySelected}
                      onClick={() => add(r)}
                      className="flex w-full items-center gap-3 px-3 py-2 text-left hover:bg-[color:var(--background)] disabled:opacity-40"
                    >
                      {r.thumbnailUrl ? (
                        // eslint-disable-next-line @next/next/no-img-element
                        <img
                          src={r.thumbnailUrl}
                          alt=""
                          className="h-8 w-8 rounded object-cover"
                        />
                      ) : (
                        <div className="h-8 w-8 rounded bg-[color:var(--background)]" />
                      )}
                      <span className="flex-1 truncate text-sm text-foreground">
                        {r.title}
                      </span>
                      {alreadySelected ? (
                        <span className="text-[10px] uppercase tracking-wide text-foreground-secondary">
                          Added
                        </span>
                      ) : null}
                    </button>
                  </li>
                );
              })}
            </ul>
          ) : null}
        </div>
      ) : null}

      {atMax ? (
        <p className="text-xs text-foreground-secondary">
          Maximum of {max} products reached. Remove one to add another.
        </p>
      ) : null}
    </div>
  );
}

interface AdminProductApiResponse {
  data?: Array<{
    handle: string;
    title: string;
    media?: Array<{ url: string }>;
  }>;
}

async function fetchProducts(
  storeId: string,
  search: string,
  signal?: AbortSignal,
): Promise<ProductOption[]> {
  const url = `/api/admin/stores/${encodeURIComponent(storeId)}/products?search=${encodeURIComponent(search)}&page_size=20`;
  const res = await fetch(url, { signal });
  if (!res.ok) return [];
  const body = (await res.json()) as AdminProductApiResponse;
  const rows = body.data ?? [];
  return rows.map((p) => ({
    slug: p.handle,
    title: p.title,
    thumbnailUrl: p.media?.[0]?.url ?? null,
  }));
}

// Resolve a known slug list one-by-one via the search endpoint. Each
// slug is issued as a search query; the returned product's handle is
// compared exactly to filter out fuzzy matches. Used for hydrating
// chip labels and detecting renamed/deleted products.
async function fetchProductsBySlugs(
  storeId: string,
  slugs: string[],
): Promise<ProductOption[]> {
  const found: ProductOption[] = [];
  await Promise.all(
    slugs.map(async (slug) => {
      const items = await fetchProducts(storeId, slug).catch(() => []);
      const match = items.find((i) => i.slug === slug);
      if (match) found.push(match);
    }),
  );
  return found;
}
