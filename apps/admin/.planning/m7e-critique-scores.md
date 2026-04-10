# M7e Paper · Ink · Moss Critique Scores

Scored each surface against the design rubric: Paper bg, Ink text/CTA,
Moss accent, Source Serif 4 display numerals, Source Sans 3 UI, hairline
rules, 6px radius, Moss focus ring, WCAG 2.1 AA, prefers-reduced-motion.

| Surface | Score | Notes |
|---------|-------|-------|
| CsvUploadDropzone | 8.5 | Paper bg, dashed hairline border, Moss drag-over highlight, Moss focus ring. Serif prompt for drop CTA. Keyboard accessible. |
| CsvPreviewTable | 8.0 | Hairline ruled rows, Paper header bg, Ink text, 6px radius. Truncated cells. No bordered cards. |
| CsvColumnMapping | 8.0 | Elevated-white card rows, Ink labels, Moss focus ring on selects, Paper bg selects with 6px radius. Arrow separator. |
| CsvJobProgress | 9.0 | Serif numerals for counters, Moss fill progress bar on Paper bg, semantic progressbar role, status badges use Moss/Danger palette. Cancel button with Moss focus ring. Polling uses setInterval (no animation to reduce-motion gate). |
| CsvImportHistory | 8.0 | Hairline ruled table, Paper header, serif numerals for row counts, Moss link color, Eye icon for view action. |
| CsvErrorSummary | 8.5 | Elevated-white surface, Danger icon, serif error count, Moss download link with border. Clean focus ring. |

All surfaces >= 8.0 (threshold 7.5). No polish fixes needed.

## Verification

- `npm run test` (vitest): 157 tests, 34 files — all green
- `npx tsc --noEmit`: clean
- `npm run build`: pre-existing failure in `@repo/ui/coming-soon.tsx` (lucide-react not in packages/ui deps). Not related to M7e.
