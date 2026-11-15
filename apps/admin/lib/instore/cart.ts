// The in-store sale cart (#1105). Pure, so the sale screen's rules are
// testable without React: what may change when, and the idempotency key
// that makes "Paid" safe to retry.
//
// The total the seller charges always comes from the server's quote; the
// subtotal here is only the running figure shown while ringing up.

export type CartLine = {
  key: string;
  variantId?: string;
  title: string;
  optionSummary?: string;
  imageUrl?: string;
  unitPrice: string;
  quantity: number;
  custom?: boolean;
};

export type Cart = {
  lines: CartLine[];
  discount: string;
  note: string;
  // Created when the seller first charges; kept until the sale is recorded,
  // refused, or discarded, so every retry of "Paid" is the same sale.
  idempotencyKey: string | null;
  // Set when "Paid" failed without an answer: the sale may exist, so the
  // cart must not change until it is confirmed or discarded.
  locked: boolean;
};

export type CartAction =
  | { type: "add"; line: Omit<CartLine, "key" | "quantity"> }
  | { type: "setQty"; key: string; quantity: number }
  | { type: "remove"; key: string }
  | { type: "discount"; value: string }
  | { type: "note"; value: string }
  | { type: "beginCharge" }
  | { type: "uncertain" }
  | { type: "refused" }
  | { type: "reset" };

export function emptyCart(): Cart {
  return { lines: [], discount: "", note: "", idempotencyKey: null, locked: false };
}

export function cartReducer(cart: Cart, action: CartAction): Cart {
  if (cart.locked && action.type !== "reset" && action.type !== "refused" && action.type !== "uncertain") {
    return cart;
  }
  switch (action.type) {
    case "add": {
      const same = !action.line.custom && cart.lines.find((l) => !l.custom && l.variantId === action.line.variantId);
      if (same) {
        return { ...cart, lines: cart.lines.map((l) => (l === same ? { ...l, quantity: l.quantity + 1 } : l)) };
      }
      return { ...cart, lines: [...cart.lines, { ...action.line, key: crypto.randomUUID(), quantity: 1 }] };
    }
    case "setQty":
      if (action.quantity < 1) return { ...cart, lines: cart.lines.filter((l) => l.key !== action.key) };
      return { ...cart, lines: cart.lines.map((l) => (l.key === action.key ? { ...l, quantity: action.quantity } : l)) };
    case "remove":
      return { ...cart, lines: cart.lines.filter((l) => l.key !== action.key) };
    case "discount":
      return { ...cart, discount: action.value };
    case "note":
      return { ...cart, note: action.value };
    case "beginCharge":
      return cart.idempotencyKey ? cart : { ...cart, idempotencyKey: crypto.randomUUID() };
    case "uncertain":
      return { ...cart, locked: true };
    case "refused":
      // Definitely not recorded: the next charge is a new attempt.
      return { ...cart, idempotencyKey: null, locked: false };
    case "reset":
      return emptyCart();
  }
}

function toCents(amount: string): number | null {
  if (!/^\d+(\.\d{1,2})?$/.test(amount.trim())) return null;
  return Math.round(Number(amount) * 100);
}

function fromCents(cents: number): string {
  return (cents / 100).toFixed(2);
}

export function subtotal(cart: Cart): string {
  return fromCents(cart.lines.reduce((sum, l) => sum + (toCents(l.unitPrice) ?? 0) * l.quantity, 0));
}

// Change due on a cash sale, or null when tendered is not an amount or does
// not cover the total.
export function change(tendered: string, total: string): string | null {
  const t = toCents(tendered);
  const g = toCents(total);
  if (t === null || g === null || t < g) return null;
  return fromCents(t - g);
}

export function toSaleLines(
  cart: Cart,
): { variant_id?: string; custom_title?: string; custom_price?: string; quantity: number }[] {
  return cart.lines.map((l) =>
    l.custom
      ? { custom_title: l.title, custom_price: l.unitPrice, quantity: l.quantity }
      : { variant_id: l.variantId, quantity: l.quantity },
  );
}

const storageKey = (storeId: string) => `mark8ly:instore-cart:${storeId}`;

// The cart survives a reload or a dropped connection on the device it was
// rung up on, key and lock included.
export function loadCart(storeId: string): Cart {
  try {
    const raw = localStorage.getItem(storageKey(storeId));
    if (!raw) return emptyCart();
    const cart = JSON.parse(raw) as Cart;
    return Array.isArray(cart.lines) ? { ...emptyCart(), ...cart } : emptyCart();
  } catch {
    return emptyCart();
  }
}

export function saveCart(storeId: string, cart: Cart): void {
  try {
    localStorage.setItem(storageKey(storeId), JSON.stringify(cart));
  } catch {
    // Storage full or blocked: the sale still works, it just won't survive a reload.
  }
}
