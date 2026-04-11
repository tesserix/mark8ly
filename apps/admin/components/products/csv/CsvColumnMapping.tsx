"use client";

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@tesserix/web";

export const PRODUCT_FIELDS = [
  "title",
  "handle",
  "description",
  "status",
  "price",
  "sku",
  "stock",
] as const;

export type ProductField = (typeof PRODUCT_FIELDS)[number];

export type ColumnMapping = Record<string, ProductField | "">;

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
            className="flex items-center gap-4 rounded-[6px] border border-[var(--ink-900)]/5 bg-[var(--background-elevated)] px-3 py-2"
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
                    {field}
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

/**
 * Build an initial mapping by auto-matching CSV headers to product fields
 * using exact name matches (case-insensitive).
 */
export function autoMapColumns(csvHeaders: string[]): ColumnMapping {
  const mapping: ColumnMapping = {};
  const fieldSet = new Set<string>(PRODUCT_FIELDS);

  for (const csvHeader of csvHeaders) {
    const lower = csvHeader.toLowerCase().trim();
    if (fieldSet.has(lower)) {
      mapping[csvHeader] = lower as ProductField;
    }
  }

  return mapping;
}
