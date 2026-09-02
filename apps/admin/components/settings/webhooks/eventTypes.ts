// apps/admin/components/settings/webhooks/eventTypes.ts
//
// The closed set of 18 event types a subscription may select, mirrored
// from marketplace-api's allowedEventTypes (webhooks.go) and grouped by
// aggregate so the picker reads as five short lists instead of one long
// one. Keep this in sync with services/marketplace-api/internal/outbox's
// Event* constants — a mismatch here means a merchant can select a type
// the backend rejects, or can't select one it accepts.

export interface EventTypeGroup {
  aggregate: string;
  types: { value: string; label: string }[];
}

export const EVENT_TYPE_GROUPS: EventTypeGroup[] = [
  {
    aggregate: "Order",
    types: [
      { value: "order.placed", label: "Order placed" },
      { value: "order.confirmed", label: "Order confirmed" },
      { value: "order.fulfilled", label: "Order fulfilled" },
      { value: "order.partially_fulfilled", label: "Order partially fulfilled" },
      { value: "order.cancelled", label: "Order cancelled" },
      { value: "order.refunded", label: "Order refunded" },
    ],
  },
  {
    aggregate: "Return",
    types: [
      { value: "return.requested", label: "Return requested" },
      { value: "return.approved", label: "Return approved" },
      { value: "return.received", label: "Return received" },
      { value: "return.refunded", label: "Return refunded" },
      { value: "return.rejected", label: "Return rejected" },
    ],
  },
  {
    aggregate: "Product",
    types: [
      { value: "product.created", label: "Product created" },
      { value: "product.updated", label: "Product updated" },
      { value: "product.deleted", label: "Product deleted" },
    ],
  },
  {
    aggregate: "Category",
    types: [
      { value: "category.created", label: "Category created" },
      { value: "category.updated", label: "Category updated" },
      { value: "category.deleted", label: "Category deleted" },
    ],
  },
  {
    aggregate: "Cart",
    types: [
      {
        value: "abandoned_cart.recovery_email",
        label: "Abandoned cart recovery email sent",
      },
    ],
  },
];

export const ALL_EVENT_TYPES: string[] = EVENT_TYPE_GROUPS.flatMap((g) =>
  g.types.map((t) => t.value),
);

const LABELS: Record<string, string> = Object.fromEntries(
  ALL_EVENT_TYPES.map((value) => [
    value,
    EVENT_TYPE_GROUPS.flatMap((g) => g.types).find((t) => t.value === value)!.label,
  ]),
);

export function eventTypeLabel(value: string): string {
  return LABELS[value] ?? value;
}
