import { create } from "zustand";
import AsyncStorage from "@react-native-async-storage/async-storage";
import { newCartToken } from "@/lib/cart-token";

// The key name is historical. The schema version now lives in the stored
// payload (see StoredCart) so a shape change can be detected and migrated
// rather than guessed at from a key name — #964.
const STORAGE_KEY = "mark8ly_cart_v1";

// Bumped by #964, which made a line's identity its variant PLUS what the
// buyer filled in. v1 is a bare array of lines, every one of which is a
// valid v2 line, so the migration keeps carts rather than emptying them.
const CART_SCHEMA_VERSION = 2;

interface StoredCart {
  v: number;
  lines: CartLine[];
  /**
   * Added by #969 WITHOUT a version bump: it is an optional key on the
   * envelope, every v2 reader ignores keys it does not know, and a cart
   * without one simply mints a token on first use. See lib/cart-token.ts.
   */
  cartToken?: string;
}

/**
 * A line's identity, as an opaque token. Branded for the reason the web
 * one is: a raw variantId is also a string, so without the brand every
 * pre-#964 call site compiles and silently matches nothing.
 */
export type CartLineKey = string & { readonly __cartLineKey: unique symbol };

/** One answer the buyer gave to a personalisation field (#962). */
export interface CartLinePersonalisation {
  fieldId: string;
  uploadId?: string;
  text?: string;
  optionId?: string;
  checked?: boolean;
  /**
   * Display-only snapshots taken at add-to-cart, as on web (#966). The
   * cart renders from AsyncStorage with no access to the product's field
   * definitions, so without these it can show a value but not what it is
   * FOR. DELIBERATELY OUTSIDE lineKey, which is an explicit positional
   * tuple: a label is not part of a line's identity, and folding it in
   * would change every existing key and stop quantity merging for carts
   * already on disk.
   */
  fieldLabel?: string;
  optionLabel?: string;
}

/**
 * The identity of a cart line.
 *
 * This store used to key on variantId alone, which cannot represent two
 * personalisations of one variant — a mug printed "Asha" and the same mug
 * printed "Ravi" are one variant and two lines. With no personalisation
 * the fingerprint is empty and the key reduces to the variant, so nothing
 * changes for products that ask the buyer for nothing.
 */
export function lineKey(
  line: Pick<CartLine, "productId" | "variantId" | "personalisation">,
): CartLineKey {
  const entries = line.personalisation ?? [];
  const canonical = entries
    .map((e) => [
      e.fieldId,
      e.uploadId ?? "",
      e.text ?? "",
      e.optionId ?? "",
      e.checked === undefined ? "" : String(e.checked),
    ])
    .sort((a, b) => (a[0]! < b[0]! ? -1 : a[0]! > b[0]! ? 1 : 0));
  return JSON.stringify([
    line.productId,
    line.variantId,
    entries.length === 0 ? "" : JSON.stringify(canonical),
  ]) as CartLineKey;
}

export interface CartLine {
  productId: string;
  variantId: string;
  handle: string;
  title: string;
  variantTitle: string;
  unitPriceAmount: string;
  currencyCode: string;
  imageUrl: string;
  quantity: number;
  personalisation?: CartLinePersonalisation[];
}

interface CartState {
  lines: CartLine[];
  hydrated: boolean;
  /** Null until first needed. Read it through ensureCartToken. */
  cartToken: string | null;
  hydrate: () => Promise<void>;
  /**
   * The identity the server ties uploads and stock holds to (#969).
   * Minted once per cart and persisted, so an upload made on the product
   * screen is still this cart's at checkout after an app restart.
   */
  ensureCartToken: () => string;
  add: (line: Omit<CartLine, "quantity">, quantity?: number) => void;
  /** key is lineKey(line) — NOT a variant id. See lineKey (#964). */
  setQuantity: (key: CartLineKey, quantity: number) => void;
  remove: (key: CartLineKey) => void;
  clear: () => void;
  itemCount: () => number;
  subtotalAmount: () => number;
}

async function persist(lines: CartLine[], cartToken: string | null) {
  try {
    const payload: StoredCart = { v: CART_SCHEMA_VERSION, lines };
    if (cartToken) payload.cartToken = cartToken;
    await AsyncStorage.setItem(STORAGE_KEY, JSON.stringify(payload));
  } catch {
    // Best-effort persistence — losing the cart on app kill is preferable
    // to crashing the app over a storage error.
  }
}

/**
 * Local cart, persisted to AsyncStorage. For signed-in customers we
 * sync this to the server in a follow-up — for the initial launch a
 * local cart already covers the guest checkout flow.
 */
export const useCartStore = create<CartState>((set, get) => ({
  lines: [],
  hydrated: false,
  cartToken: null,

  hydrate: async () => {
    if (get().hydrated) return;
    try {
      const raw = await AsyncStorage.getItem(STORAGE_KEY);
      const parsed: unknown = raw ? JSON.parse(raw) : [];
      // A bare array is v1 and migrates as-is; the envelope is v2.
      const stored = isStoredCart(parsed) && parsed.v <= CART_SCHEMA_VERSION ? parsed : null;
      const lines: CartLine[] = Array.isArray(parsed)
        ? (parsed as CartLine[])
        : (stored?.lines ?? []);
      set({ lines, cartToken: stored?.cartToken ?? null, hydrated: true });
    } catch {
      set({ hydrated: true });
    }
  },

  ensureCartToken: () => {
    const existing = get().cartToken;
    if (existing) return existing;
    const token = newCartToken();
    set({ cartToken: token });
    persist(get().lines, token);
    return token;
  },

  add: (line, quantity = 1) => {
    const lines = get().lines.slice();
    const key = lineKey(line);
    const existing = lines.findIndex((l) => lineKey(l) === key);
    if (existing >= 0) {
      lines[existing] = { ...lines[existing]!, quantity: lines[existing]!.quantity + quantity };
    } else {
      lines.push({ ...line, quantity });
    }
    set({ lines });
    persist(lines, get().cartToken);
  },

  setQuantity: (key, quantity) => {
    const lines = get().lines
      .map((l) => (lineKey(l) === key ? { ...l, quantity } : l))
      .filter((l) => l.quantity > 0);
    set({ lines });
    persist(lines, get().cartToken);
  },

  remove: (key) => {
    const lines = get().lines.filter((l) => lineKey(l) !== key);
    set({ lines });
    persist(lines, get().cartToken);
  },

  // The token survives a clear. After an order it has already been spent
  // (holds committed, uploads claimed) and the server scopes nothing new
  // to it; keeping it costs nothing and rotating it is one more branch.
  clear: () => {
    set({ lines: [] });
    persist([], get().cartToken);
  },

  itemCount: () => get().lines.reduce((sum, l) => sum + l.quantity, 0),
  subtotalAmount: () =>
    get().lines.reduce((sum, l) => sum + l.quantity * Number(l.unitPriceAmount), 0),
}));

function isStoredCart(value: unknown): value is StoredCart {
  if (typeof value !== "object" || value === null) return false;
  const c = value as Record<string, unknown>;
  return typeof c.v === "number" && Array.isArray(c.lines);
}
