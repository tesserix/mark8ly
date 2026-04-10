"use client";

interface CsvPreviewTableProps {
  headers: string[];
  rows: string[][];
}

export function CsvPreviewTable({ headers, rows }: CsvPreviewTableProps) {
  if (headers.length === 0) {
    return (
      <p className="font-[var(--font-body)] text-sm text-[var(--ink-900)]/60 py-4">
        No preview available
      </p>
    );
  }

  return (
    <div className="overflow-x-auto rounded-[6px] border border-[var(--ink-900)]/10">
      <table className="w-full text-left font-[var(--font-body)] text-sm">
        <thead>
          <tr className="border-b border-[var(--ink-900)]/10 bg-[var(--paper-200)]">
            {headers.map((h) => (
              <th
                key={h}
                className="px-3 py-2 font-medium text-[var(--ink-900)]/70"
              >
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, i) => (
            <tr
              key={i}
              className="border-b border-[var(--ink-900)]/5 last:border-b-0"
            >
              {row.map((cell, j) => (
                <td
                  key={j}
                  className="px-3 py-2 text-[var(--ink-900)] truncate max-w-[200px]"
                >
                  {cell}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
