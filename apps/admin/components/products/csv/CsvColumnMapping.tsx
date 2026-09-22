"use client";

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@tesserix/web";

// These are the importer's canonical column names, and they must stay
// spelled exactly as the Go parser reads them — a mapper that offers a
// field the parser does not look for silently drops that column. "price"
// was such a field: the parser reads "base_price", so price never mapped
// and auto-mapping left it on "skip".
export const PRODUCT_FIELDS = [
  "title",
  "handle",
  "description",
  "status",
  "base_price",
  "sku",
  "stock",
  "weight",
  "category_slugs",
] as const;

export type ProductField = (typeof PRODUCT_FIELDS)[number];

export type ColumnMapping = Record<string, ProductField | "">;

/** Display names. The value sent to the server is always the canonical key. */
const FIELD_LABELS: Record<ProductField, string> = {
  title: "title",
  handle: "handle",
  description: "description",
  status: "status",
  base_price: "price (base_price)",
  sku: "sku",
  stock: "stock",
  weight: "weight",
  category_slugs: "categories (category_slugs)",
};

interface CsvColumnMappingProps {
  csvHeaders: string[];
  mapping: ColumnMapping;
  onMappingChange: (mapping: ColumnMapping) => void;
}

export function CsvColumnMapping({
  csvHeaders,
  mapping,
  onMappingChange,
}: CsvColumnMappingProps) {
  const handleChange = (csvHeader: string, value: string) => {
    const updated: ColumnMapping = {
      ...mapping,
      [csvHeader]: value as ProductField | "",
    };
    onMappingChange(updated);
  };

  return (
    <div className="flex flex-col gap-3">
      <h3 className="font-[var(--font-display)] text-base text-[var(--ink-900)]">
        Map columns
      </h3>
      <div className="flex flex-col gap-2">
        {csvHeaders.map((csvHeader) => (
          <div
            key={csvHeader}
            className="flex items-center gap-4 rounded-md border border-[var(--ink-900)]/5 bg-[var(--background-elevated)] px-3 py-2"
          >
            <span className="w-40 shrink-0 truncate font-[var(--font-body)] text-sm text-[var(--ink-900)]">
              {csvHeader}
            </span>
            <span
              className="font-[var(--font-body)] text-xs text-[var(--ink-900)]/40"
              aria-hidden="true"
            >
              →
            </span>
            <Select
              value={mapping[csvHeader] || "__skip__"}
              onValueChange={(value) =>
                handleChange(csvHeader, value === "__skip__" ? "" : value)
              }
            >
              <SelectTrigger
                className="flex-1"
                aria-label={`Map ${csvHeader} to product field`}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__skip__">— skip —</SelectItem>
                {PRODUCT_FIELDS.map((field) => (
                  <SelectItem key={field} value={field}>
                    {FIELD_LABELS[field]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        ))}
      </div>
    </div>
  );
}

// Header spellings common in merchant exports. Mark8ly's pitch is
// migrating off Shopify and Woo, and neither writes our column names.
const FIELD_ALIASES: Record<string, ProductField> = {
  name: "title",
  "product name": "title",
  slug: "handle",
  "url key": "handle",
  price: "base_price",
  "variant price": "base_price",
  "regular price": "base_price",
  "variant sku": "sku",
  quantity: "stock",
  qty: "stock",
  inventory: "stock",
  "variant inventory qty": "stock",
  "body (html)": "description",
  categories: "category_slugs",
};

/**
 * Build an initial mapping by matching CSV headers to product fields,
 * first by canonical name and then by common export aliases
 * (case-insensitive).
 */
export function autoMapColumns(csvHeaders: string[]): ColumnMapping {
  const mapping: ColumnMapping = {};
  const fieldSet = new Set<string>(PRODUCT_FIELDS);
  const taken = new Set<ProductField>();

  for (const csvHeader of csvHeaders) {
    const lower = csvHeader.toLowerCase().trim();
    const field = fieldSet.has(lower)
      ? (lower as ProductField)
      : FIELD_ALIASES[lower];
    // The server rejects two headers claiming one field, so first match
    // wins here rather than sending a mapping that cannot be accepted.
    if (field && !taken.has(field)) {
      mapping[csvHeader] = field;
      taken.add(field);
    }
  }

  return mapping;
}
