// How an in-store sale was paid. A plain module so server components (the
// order page, Your sales today) and client ones can both read it.

export type PaymentMethod = "card_external" | "cash" | "bank_transfer" | "other";

export const PAYMENT_METHODS: { value: PaymentMethod; label: string }[] = [
  { value: "card_external", label: "Card (EFTPOS)" },
  { value: "cash", label: "Cash" },
  { value: "bank_transfer", label: "Bank transfer" },
  { value: "other", label: "Other" },
];
