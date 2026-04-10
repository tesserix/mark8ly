# M7d Critique Scores — Paper / Ink / Moss Rubric

Scored each new surface on: token compliance, typography, spacing/rhythm,
hairline rules, focus rings, role gating, a11y, and editorial restraint.

| Surface                       | Score | Notes                                                      |
|-------------------------------|-------|------------------------------------------------------------|
| CopyToStoreDialog             | 8.0   | Paper tokens, serif title, Moss focus ring, radio list. Hairline rule between media toggle and form. Dialog via @tesserix/web. |
| BulkActionsBar                | 8.0   | --paper-200 bg, hairline top rule, Moss focus rings, role-gated (staff/viewer hidden). Danger variant uses --danger token. No shadows beyond --shadow-1. |
| BulkDeleteConfirmDialog       | 8.5   | Uses @tesserix/web AlertDialog. Minimal surface. Count in message. No hex values. |
| BulkCategoryAssignPopover     | 7.5   | Reuses M7b ProductCategoriesPicker. --background-elevated card, --shadow-2. Hairline rule above apply button. Meets threshold. |

All surfaces >= 7.5. No polish fixes required.

## Token compliance
- Zero new hex values introduced
- All colors via var(--ink-900), var(--paper-200), var(--moss-700), var(--signal), var(--danger)
- Source Serif 4 for dialog titles, Source Sans 3 for body/UI
- 6px radius (rounded-md) default
- Moss focus rings on all interactive elements

## Accessibility
- All checkboxes/radios have aria-labels
- Bulk bar has role="toolbar" and aria-label
- Dialog uses semantic heading hierarchy
- Focus visible outlines use --moss-700
