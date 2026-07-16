# Mobile-admin Product Editor Redesign — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Redesign the product edit + create screens: add-option-that-creates-variants, a category picker sheet, a streamlined create flow, collapsible variants, and an editorial layout pass.

**Architecture:** Client-only (Expo/React Native). The one dangerous piece is `buildOptionMatrix`, a pure helper that turns "add an option" into the full `variants` matrix the backend needs — built and exhaustively tested in isolation before anything consumes it. Bottom sheets use `@gorhom/bottom-sheet` (already installed + provider mounted). No backend changes, no new deps.

**Tech Stack:** Expo 56 / React Native, TypeScript, zod 4, @tanstack/react-query, @gorhom/bottom-sheet v5.2.8, expo-haptics, jest + @testing-library/react-native.

**Spec:** `docs/superpowers/specs/2026-07-16-mobile-admin-product-editor-redesign.md` — read it; this plan implements it.

## Global Constraints

- **NEVER** `npm ci` / `npm install` / `rm -rf node_modules`. Every dep is installed. Need a new one → **stop and ask**.
- Never touch anything in `node_modules/`. Don't modify `metro.config.js`/`tsconfig.json`/`jest.config.js`/`babel.config.js`/tailwind/`app.config.js`/`eas.json`.
- **tsc gate:** `npx tsc --noEmit --pretty false 2>&1 | grep -c "error TS"` must print `0` in BOTH `apps/mobile-admin` AND `packages/mobile-shared`. `--pretty false` MANDATORY. Count; never grep by filename.
- **jest summary is coloured — read the TAIL.** Single-file `npx jest <file>` hangs without `--forceExit`.
- **Component tests that render anything pulling `@/components/ui` MUST include the `jest.mock("lucide-react-native", ...)` stub** from `__tests__/security.test.tsx` (barrel pulls lucide ESM jest can't parse).
- **`noUnusedLocals` is OFF** — tsc won't catch a dangling import after an extraction/edit. Check by eye.
- Design system: **only `apps/mobile-admin/lib/theme.ts` tokens** — no hardcoded colours/radii/spacing. Compose `@/components/ui` primitives. Paper·Ink·Moss, one moss accent per view, WCAG AA (44pt targets, SR labels, honor reduced motion via `AccessibilityInfo`/reduced-motion).
- 🔴 **The `variants` array on a product PATCH is a FULL DESIRED MATRIX** — `applyVariantsDiff` soft-deletes any existing variant not present. NEVER send a `variants` body except the output of `buildOptionMatrix`, which preserves existing variants by id.
- Baseline before Task 1: jest **263/263**, both tsc **0** (plus the committed padding fix `8a1f7a3c`).
- Commit directly to `main`, single-line conventional messages, no signatures, no PRs.
- Working assumptions (user-approved defaults, correctable): new option combinations inherit the first existing variant's **price** + **inventory_quantity 0**; ambiguous multi-axis re-expansions **fail loud** (throw a named error surfaced as an Alert) rather than guess.

---

### Task 1: `FieldInput` / `FieldLabel` shared primitives

Kill the input-styling drift (`[id].tsx`, `new.tsx`, `VariantEditor`, `OptionsEditor` each redefine an input; two different backgrounds). One primitive, standard on `surfaceAlt`.

**Files:**
- Create: `apps/mobile-admin/components/ui/FieldInput.tsx`
- Modify: `apps/mobile-admin/components/ui/index.ts` (export it)
- Test: `apps/mobile-admin/__tests__/field-input.test.tsx`

**Interfaces:**
- Produces: `<FieldLabel label={string} />`; `<FieldInput label?={string} value onChangeText onBlur? placeholder? multiline? accessibilityLabel? keyboardType? autoCapitalize? />` — a labelled text input styled on `theme.colors.surfaceAlt`, hairline border, `radii.md`, 44pt min height (multiline taller). `FieldInput` forwards all standard `TextInput` props.

- [ ] **Step 1: Write the failing test**

```tsx
import { render, fireEvent } from "@testing-library/react-native";
import { FieldInput, FieldLabel } from "@/components/ui/FieldInput";

jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

describe("FieldInput", () => {
  it("renders its label and value and reports changes", () => {
    const onChangeText = jest.fn();
    const { getByLabelText, getByText } = render(
      <FieldInput label="Title" value="Hat" onChangeText={onChangeText} accessibilityLabel="Title" />,
    );
    expect(getByText("Title")).toBeTruthy();
    fireEvent.changeText(getByLabelText("Title"), "Cap");
    expect(onChangeText).toHaveBeenCalledWith("Cap");
  });

  it("fires onBlur", () => {
    const onBlur = jest.fn();
    const { getByLabelText } = render(
      <FieldInput value="x" onChangeText={() => {}} onBlur={onBlur} accessibilityLabel="F" />,
    );
    fireEvent(getByLabelText("F"), "blur");
    expect(onBlur).toHaveBeenCalled();
  });

  it("FieldLabel renders caption text", () => {
    const { getByText } = render(<FieldLabel label="SKU" />);
    expect(getByText("SKU")).toBeTruthy();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/mobile-admin && npx jest __tests__/field-input.test.tsx --forceExit 2>&1 | tail -8`
Expected: FAIL — `Cannot find module '@/components/ui/FieldInput'`.

- [ ] **Step 3: Write minimal implementation**

Read an existing `[id].tsx` `styles.input` + `FieldLabel` first to match tone. Create `apps/mobile-admin/components/ui/FieldInput.tsx`:

```tsx
import { TextInput, View, StyleSheet, type TextInputProps } from "react-native";
import { Text } from "./Text";
import { theme } from "@/lib/theme";

export function FieldLabel({ label }: { label: string }) {
  return (
    <Text preset="caption" color="textTertiary">
      {label}
    </Text>
  );
}

interface FieldInputProps extends TextInputProps {
  label?: string;
}

/**
 * The one text input for the product forms. Standardised on surfaceAlt so
 * create and edit can't drift again (they each used to redefine styles.input,
 * one on elevated and one on surfaceAlt).
 */
export function FieldInput({ label, style, multiline, ...rest }: FieldInputProps) {
  return (
    <View style={styles.wrap}>
      {label ? <FieldLabel label={label} /> : null}
      <TextInput
        style={[styles.input, multiline ? styles.multiline : null, style]}
        placeholderTextColor={theme.colors.textTertiary}
        multiline={multiline}
        textAlignVertical={multiline ? "top" : "auto"}
        {...rest}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: { gap: theme.spacing.xs },
  input: {
    minHeight: 44,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    borderRadius: theme.radii.md,
    paddingHorizontal: theme.spacing.sm,
    color: theme.colors.text,
    backgroundColor: theme.colors.surfaceAlt,
  },
  multiline: { minHeight: 96, paddingTop: theme.spacing.sm },
});
```

Add to `apps/mobile-admin/components/ui/index.ts`:

```ts
export { FieldInput, FieldLabel } from "./FieldInput";
```

**Do not** rip out the existing inline inputs in this task — later tasks migrate their own screens. This task only introduces the primitive.

- [ ] **Step 4: Run tests + gates**

Run: `cd apps/mobile-admin && npx jest __tests__/field-input.test.tsx --forceExit 2>&1 | tail -6` → PASS.
Both tsc gates → 0 and 0.

- [ ] **Step 5: Commit**

```bash
git add apps/mobile-admin/components/ui/FieldInput.tsx apps/mobile-admin/components/ui/index.ts apps/mobile-admin/__tests__/field-input.test.tsx
git commit -m "feat(mobile-admin): add shared FieldInput/FieldLabel primitives"
```

---

### Task 2: `buildOptionMatrix` — the dangerous core (pure helper, exhaustively tested)

Turns a desired option set into the full `variants` matrix to PATCH, **preserving existing variants by id** so the full-matrix contract never soft-deletes real data. No UI in this task — pure function + tests only.

**Backend truth (verified):** `PATCH {options}` alone creates option rows but NO variants and leaves the existing variant unlinked. To add an axis meaningfully you must send `options` + a `variants` matrix; `variants` is a full desired set (`applyVariantsDiff` soft-deletes anything omitted). Each variant carries `option_values: [{option_name, value}]`.

**Files:**
- Create: `apps/mobile-admin/lib/option-matrix.ts`
- Test: `apps/mobile-admin/__tests__/option-matrix.test.tsx`

**Interfaces:**
- Consumes: `ProductDetail`, `ProductVariant`, `ProductOption` (from `@repo/mobile-shared/api/schemas/products`); `UpdateProductOptionBody` and the variant body shape from `@repo/mobile-shared/api/products`.
- Produces:
  - `type OptionMatrixResult = { options: UpdateProductOptionBody[]; variants: VariantMatrixInput[] }`
  - `VariantMatrixInput = { id?: string; sku: string; price: number; inventory_quantity: number; currency_code: string; option_values: { option_name: string; value: string }[] }`
  - `buildOptionMatrix(product: ProductDetail, desiredOptions: UpdateProductOptionBody[]): OptionMatrixResult` — returns the options + the full Cartesian variant matrix, reusing each existing variant (by id, price, stock, sku) wherever its option tuple survives; new combinations get a derived SKU, the first existing variant's price, and `inventory_quantity: 0`.
  - It **throws** `OptionMatrixError` (a named Error subclass, also exported) when the mapping is ambiguous (e.g. two existing variants collide onto one tuple, or an existing variant's tuple can't be resolved in the desired space).

**Note:** This task needs a `variants` field on `UpdateProductBody`. It does NOT exist (deliberately removed earlier). Add it **typed narrowly** so only this helper's output can populate it — see Step 3.

- [ ] **Step 1: Write the failing test**

```tsx
import { buildOptionMatrix, OptionMatrixError } from "@/lib/option-matrix";

// Minimal ProductDetail-shaped fixtures. Only the fields the helper reads.
const singleVariantProduct = {
  id: "p1",
  title: "Linen Shirt",
  options: [],
  variants: [
    {
      id: "v1",
      sku: "TBS-LS",
      price: 149,
      inventory_quantity: 5,
      currency_code: "AUD",
      option_values: [],
      position: 0,
    },
  ],
} as never;

describe("buildOptionMatrix — single variant + one new axis", () => {
  it("maps the existing variant onto the FIRST value and creates the rest empty", () => {
    const { options, variants } = buildOptionMatrix(singleVariantProduct, [
      { name: "Size", values: ["S", "M", "L"] },
    ]);
    expect(options).toEqual([{ name: "Size", values: ["S", "M", "L"] }]);
    expect(variants).toHaveLength(3);
    // First value reuses the existing variant id + its price/stock/sku.
    const s = variants.find((v) => v.option_values[0]!.value === "S")!;
    expect(s.id).toBe("v1");
    expect(s.price).toBe(149);
    expect(s.inventory_quantity).toBe(5);
    expect(s.sku).toBe("TBS-LS");
    // New combinations: no id, inherit price, 0 stock.
    const m = variants.find((v) => v.option_values[0]!.value === "M")!;
    expect(m.id).toBeUndefined();
    expect(m.price).toBe(149);
    expect(m.inventory_quantity).toBe(0);
    expect(m.sku).not.toBe("");
    expect(m.option_values).toEqual([{ option_name: "Size", value: "M" }]);
  });

  it("🔴 never drops the existing variant's id when its tuple survives", () => {
    const { variants } = buildOptionMatrix(singleVariantProduct, [
      { name: "Size", values: ["S", "M"] },
    ]);
    const withId = variants.filter((v) => v.id === "v1");
    expect(withId).toHaveLength(1); // exactly one carries the real id → no soft-delete
  });
});

describe("buildOptionMatrix — multi-axis Cartesian", () => {
  const sizedProduct = {
    id: "p2",
    title: "Tee",
    options: [{ id: "o1", name: "Size", position: 0, values: [
      { id: "s", value: "S", position: 0 }, { id: "m", value: "M", position: 1 },
    ] }],
    variants: [
      { id: "vs", sku: "T-S", price: 20, inventory_quantity: 3, currency_code: "AUD",
        option_values: [{ option_name: "Size", option_value_id: "s", value: "S" }], position: 0 },
      { id: "vm", sku: "T-M", price: 20, inventory_quantity: 4, currency_code: "AUD",
        option_values: [{ option_name: "Size", option_value_id: "m", value: "M" }], position: 1 },
    ],
  } as never;

  it("expands Size×Colour to 4, preserving both existing variants by tuple", () => {
    const { variants } = buildOptionMatrix(sizedProduct, [
      { name: "Size", values: ["S", "M"] },
      { name: "Colour", values: ["Red", "Blue"] },
    ]);
    expect(variants).toHaveLength(4);
    // The two existing variants keep their ids on their (Size, first-Colour) tuples.
    expect(variants.filter((v) => v.id === "vs" || v.id === "vm")).toHaveLength(2);
    // Every variant carries a full 2-axis tuple.
    for (const v of variants) expect(v.option_values).toHaveLength(2);
    // No id appears twice (no soft-delete via duplicate reuse).
    const ids = variants.map((v) => v.id).filter(Boolean);
    expect(new Set(ids).size).toBe(ids.length);
  });
});

describe("buildOptionMatrix — fail loud", () => {
  it("throws OptionMatrixError rather than guessing when values are empty", () => {
    expect(() => buildOptionMatrix(singleVariantProduct, [{ name: "Size", values: [] }])).toThrow(
      OptionMatrixError,
    );
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/mobile-admin && npx jest __tests__/option-matrix.test.tsx --forceExit 2>&1 | tail -10`
Expected: FAIL — `Cannot find module '@/lib/option-matrix'`.

- [ ] **Step 3: Write minimal implementation**

First widen the request body. In `packages/mobile-shared/api/products.ts`, add a narrow variant-matrix type and a `variants` field to `UpdateProductBody`:

```ts
/**
 * A variant as sent inside an option-matrix PATCH. Only produced by
 * buildOptionMatrix — never hand-constructed — because `variants` is a full
 * desired matrix that soft-deletes anything omitted. `id` present = preserve an
 * existing variant; absent = a new combination.
 */
export interface VariantMatrixInput {
  id?: string;
  sku: string;
  price: number;
  inventory_quantity: number;
  currency_code: string;
  option_values: { option_name: string; value: string }[];
}
```

and add to `UpdateProductBody`:

```ts
  /**
   * ONLY set from buildOptionMatrix's output, and only alongside `options`.
   * A full desired matrix — applyVariantsDiff soft-deletes anything missing.
   */
  variants?: VariantMatrixInput[];
```

Create `apps/mobile-admin/lib/option-matrix.ts`:

```ts
import type {
  ProductDetail,
  ProductVariant,
} from "@repo/mobile-shared/api/schemas/products";
import type {
  UpdateProductOptionBody,
  VariantMatrixInput,
} from "@repo/mobile-shared/api/products";
import { deriveSku } from "@/lib/product-display";

export class OptionMatrixError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "OptionMatrixError";
  }
}

export interface OptionMatrixResult {
  options: UpdateProductOptionBody[];
  variants: VariantMatrixInput[];
}

/** Cartesian product of the option value lists, in declared order. */
function cartesian(options: UpdateProductOptionBody[]): { option_name: string; value: string }[][] {
  return options.reduce<{ option_name: string; value: string }[][]>(
    (acc, opt) =>
      acc.flatMap((tuple) => opt.values.map((value) => [...tuple, { option_name: opt.name, value }])),
    [[]],
  );
}

/** Stable key for a tuple, order-independent within a fixed axis order. */
function tupleKey(tuple: { option_name: string; value: string }[]): string {
  return tuple.map((t) => `${t.option_name}=${t.value}`).join("|");
}

/** The tuple an existing variant occupies, projected onto the DESIRED axes. */
function existingTupleKey(
  variant: ProductVariant,
  desired: UpdateProductOptionBody[],
): string | null {
  const byName = new Map(variant.option_values.map((ov) => [ov.option_name, ov.value]));
  const tuple: { option_name: string; value: string }[] = [];
  for (const opt of desired) {
    const value = byName.get(opt.name);
    if (value === undefined) return null; // new axis this variant predates → maps to first value
    if (!opt.values.includes(value)) return null; // its value was removed
    tuple.push({ option_name: opt.name, value });
  }
  return tupleKey(tuple);
}

/**
 * Turn a desired option set into the full variants matrix to PATCH, preserving
 * existing variants by id wherever their tuple survives. See the spec's Area 1.
 * Throws OptionMatrixError on anything ambiguous rather than risk a wrong matrix.
 */
export function buildOptionMatrix(
  product: ProductDetail,
  desiredOptions: UpdateProductOptionBody[],
): OptionMatrixResult {
  if (desiredOptions.length === 0) {
    throw new OptionMatrixError("At least one option is required.");
  }
  for (const opt of desiredOptions) {
    if (opt.values.length === 0) {
      throw new OptionMatrixError(`Option "${opt.name}" needs at least one value.`);
    }
  }

  const tuples = cartesian(desiredOptions);
  const first = product.variants[0];
  const currency = first?.currency_code ?? "AUD";
  const inheritPrice = first?.price ?? 0;

  // Map each existing variant to a desired tuple key (if it still exists).
  const existingByTuple = new Map<string, ProductVariant>();
  const unmapped: ProductVariant[] = [];
  for (const v of product.variants) {
    const key = existingTupleKey(v, desiredOptions);
    if (key === null) {
      unmapped.push(v);
      continue;
    }
    if (existingByTuple.has(key)) {
      throw new OptionMatrixError("Two variants map to the same option combination.");
    }
    existingByTuple.set(key, v);
  }

  // Variants that don't project cleanly (predate a new axis) fold onto the
  // first tuple in declared order — the common single-variant case.
  if (unmapped.length > 1) {
    throw new OptionMatrixError("Can't safely re-map this product's variants on mobile.");
  }
  const firstKey = tupleKey(tuples[0]!);
  if (unmapped.length === 1 && !existingByTuple.has(firstKey)) {
    existingByTuple.set(firstKey, unmapped[0]!);
  }

  const variants: VariantMatrixInput[] = tuples.map((tuple) => {
    const key = tupleKey(tuple);
    const existing = existingByTuple.get(key);
    if (existing) {
      return {
        id: existing.id,
        sku: existing.sku,
        price: existing.price,
        inventory_quantity: existing.inventory_quantity,
        currency_code: existing.currency_code,
        option_values: tuple,
      };
    }
    const suffix = tuple.map((t) => t.value).join("-");
    return {
      sku: `${deriveSku(product.title)}-${suffix}`.toUpperCase(),
      price: inheritPrice,
      inventory_quantity: 0,
      currency_code: currency,
      option_values: tuple,
    };
  });

  return { options: desiredOptions, variants };
}
```

If `deriveSku`'s real signature differs (read `lib/product-display.ts`), adapt the call.

- [ ] **Step 4: Run tests + gates**

Run: `cd apps/mobile-admin && npx jest __tests__/option-matrix.test.tsx --forceExit 2>&1 | tail -8` → PASS.
Both tsc gates → 0 and 0.

- [ ] **Step 5: Commit**

```bash
git add apps/mobile-admin/lib/option-matrix.ts apps/mobile-admin/__tests__/option-matrix.test.tsx packages/mobile-shared/api/products.ts
git commit -m "feat(mobile-admin): buildOptionMatrix — safe variant matrix for option edits"
```

---

### Task 3: Options empty state + `OptionBuilderSheet`

Turn the blank Options card into a real feature: an empty state and a bottom sheet to add an axis, which PATCHes `options` + `buildOptionMatrix`'s variants together.

**Files:**
- Create: `apps/mobile-admin/components/products/OptionBuilderSheet.tsx`
- Modify: `apps/mobile-admin/components/products/OptionsEditor.tsx` (empty state + "＋ Add option" row + open sheet)
- Modify: `apps/mobile-admin/app/(tabs)/products/[id].tsx` (wire the add-option handler to `buildOptionMatrix`)
- Test: `apps/mobile-admin/__tests__/option-builder-sheet.test.tsx`, extend `__tests__/options-editor.test.tsx`

**Interfaces:**
- Consumes: `buildOptionMatrix` (Task 2); `@gorhom/bottom-sheet` (`BottomSheetModal`).
- Produces: `<OptionBuilderSheet ref onSubmit={(option: UpdateProductOptionBody) => void} />` (imperative `present()`); `OptionsEditor` gains an `onAddOption(option: UpdateProductOptionBody)` prop; `[id].tsx` gains `handleAddOption` that calls `updateMutation.mutate({ id, body: buildOptionMatrix(product, [...product.options.map(toReq), option]) })` wrapped in try/catch → `alertOnError`/`OptionMatrixError` alert.

- [ ] **Step 1: Write the failing test**

```tsx
import { render, fireEvent, waitFor } from "@testing-library/react-native";
import { OptionsEditor } from "@/components/products/OptionsEditor";

jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

describe("OptionsEditor empty state", () => {
  it("shows guidance and an add-option affordance when there are no options", () => {
    const { getByText, getByLabelText } = render(
      <OptionsEditor options={[]} onChange={() => {}} onAddOption={() => {}} />,
    );
    expect(getByText(/one variant/i)).toBeTruthy();
    expect(getByLabelText("Add an option")).toBeTruthy();
  });
});
```

(The sheet's own present/submit test goes in `option-builder-sheet.test.tsx`; because sheets render through a portal, test the builder's pure submit logic — name+values → `onSubmit({name, values})`, blank/duplicate suppression — the same way `OptionsEditor`'s value logic is tested, extracting a pure `buildOptionSubmission(name, values)` if needed.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/mobile-admin && npx jest __tests__/options-editor.test.tsx --forceExit 2>&1 | tail -10`
Expected: FAIL — `onAddOption` not a prop / empty-state text absent.

- [ ] **Step 3: Write minimal implementation**

Read `OptionsEditor.tsx` and the existing `app/_layout.tsx` BottomSheetModalProvider mount first. Add to `OptionsEditor`:
- an `onAddOption: (option: UpdateProductOptionBody) => void` prop;
- when `options.length === 0`, render the inline empty state (two `Text` lines per spec) instead of nothing;
- always render a full-width "＋ Add option" `Pressable` (44pt, `theme.colors.border` hairline, `Plus` from lucide in `theme.colors.accent`, `accessibilityLabel="Add an option"`) that calls a passed `onRequestAddOption` (or opens the sheet via a ref held in `[id].tsx`).

Create `OptionBuilderSheet.tsx` using `BottomSheetModal` (snapPoints `['60%']`), a Name `FieldInput`, a values chip-input (reuse OptionsEditor's chip visual), the consequence note (`surfaceAlt` ghost block + `AlertTriangle` in `theme.colors.warning`), and a sticky footer (ghost Cancel + moss "Add option", disabled until name + ≥1 value). On confirm call `onSubmit({ name, values })` and dismiss. Extract a pure `buildOptionSubmission(name: string, values: string[]): UpdateProductOptionBody | null` (trims, dedupes, returns null if invalid) and unit-test it.

In `[id].tsx`, add `handleAddOption`:

```tsx
const handleAddOption = useCallback(
  (option: UpdateProductOptionBody) => {
    try {
      const existing = toOptionRequestBodies(product.options); // existing response→request shape
      const result = buildOptionMatrix(product, [...existing, option]);
      updateMutation.mutate(
        { id, body: { options: result.options, variants: result.variants } },
        alertOnError("Failed to add option. Please try again."),
      );
    } catch (err) {
      Alert.alert("Can't add option", getErrorMessage(err, "That option can't be added safely."));
    }
  },
  [id, product, updateMutation],
);
```

Wire `onAddOption={handleAddOption}` into `<OptionsEditor>`. Import `buildOptionMatrix`, `OptionMatrixError`, `toOptionRequestBodies`.

- [ ] **Step 4: Run tests + gates**

Run: `cd apps/mobile-admin && npx jest __tests__/options-editor.test.tsx __tests__/option-builder-sheet.test.tsx --forceExit 2>&1 | tail -8` → PASS.
Run the FULL suite (`[id].tsx` changed) → all pass. Both tsc gates → 0.

- [ ] **Step 5: Commit**

```bash
git add apps/mobile-admin/components/products/OptionBuilderSheet.tsx apps/mobile-admin/components/products/OptionsEditor.tsx "apps/mobile-admin/app/(tabs)/products/[id].tsx" apps/mobile-admin/__tests__/options-editor.test.tsx apps/mobile-admin/__tests__/option-builder-sheet.test.tsx
git commit -m "feat(mobile-admin): add product options from an empty state via a builder sheet"
```

---

### Task 4: `CategoryField` + `CategoryPickerSheet`

Replace the inline checkbox list with a tappable field of selected chips that opens a searchable tree sheet.

**Files:**
- Create: `apps/mobile-admin/components/products/CategoryField.tsx`, `apps/mobile-admin/components/products/CategoryPickerSheet.tsx`
- Modify: `apps/mobile-admin/app/(tabs)/products/[id].tsx` (swap `CategoryPicker` for `CategoryField` + sheet)
- Test: `apps/mobile-admin/__tests__/category-field.test.tsx`
- Keep `CategoryPicker.tsx` + `sortCategoryTree` (the sheet reuses `sortCategoryTree`).

**Interfaces:**
- Consumes: `sortCategoryTree` (from `CategoryPicker.tsx` — export it if not already), `Category`/`CategoryRef`.
- Produces: `<CategoryField categories selected onChange />` (renders chips/placeholder, opens the sheet); `<CategoryPickerSheet ref categories selected onApply={(ids: string[]) => void} />` (`BottomSheetModal`, search + tree, staged selection committed on Done). A pure `filterTree(nodes, query)` helper returns matches-only when query non-empty.

- [ ] **Step 1: Write the failing test** — `CategoryField` shows "Add categories" when empty, shows N chips when selected, and `filterTree` returns only matching nodes for a query (fixture out of order to prove the filter). (Full code omitted here for brevity in this planning excerpt — the implementer writes render+fireEvent tests mirroring `category-picker.test.tsx`, plus a pure `filterTree` test with an out-of-order fixture.)

- [ ] **Step 2: Run — FAIL** (module missing).

- [ ] **Step 3: Implement** per spec Area 2. `CategoryPickerSheet` uses `BottomSheetModal` snap `['92%']`, `SearchField`, `BottomSheetFlatList` of `sortCategoryTree` rows (depth indent, moss `Check` when selected), staged local selection, commit one `onApply(ids)` on Done (tree order preserved via `nodes.map().filter()`). `CategoryField` shows chips (reuse chip visual, cap at 4 + "+N") or "Add categories" + chevron; opens the sheet. In `[id].tsx` replace `<CategoryPicker>` with `<CategoryField categories={categories ?? []} selected={product.categories} onChange={handleCategoriesChange} />`. Loading/empty/error states per spec.

- [ ] **Step 4: Tests + FULL suite + both tsc gates → 0.**

- [ ] **Step 5: Commit** `feat(mobile-admin): category picker sheet with search and tree`

---

### Task 5: Streamlined create screen + hand-off + `CreateNextStepsBanner`

**Files:**
- Rewrite: `apps/mobile-admin/app/(tabs)/products/new.tsx` (single screen, no steps)
- Create: `apps/mobile-admin/components/products/CreateNextStepsBanner.tsx`
- Modify: `apps/mobile-admin/app/(tabs)/products/[id].tsx` (show banner when `?created=1`, ScrollView ref + section offsets)
- Test: `apps/mobile-admin/__tests__/new-product.test.tsx`, `__tests__/create-next-steps-banner.test.tsx`

**Interfaces:**
- Consumes: `useCreateProduct` (returns the full product incl. `id`), `FieldInput`, `SegmentedControl`, `CategoryField` (Task 4), `deriveSku`.
- Produces: a single-screen create form; on success `router.replace('/(tabs)/products/[id]?created=1')` with the new id; `<CreateNextStepsBanner title onDismiss onJump={(section) => void} />`.

- [ ] **Step 1–5** per spec Area 4: delete `StepDots`/step state/footer; Essentials + Status(`SegmentedControl`) + optional Category; sticky moss "Create product" clearing the dock; inline validation (title + non-negative price) not Alert; `router.replace` into edit; banner on `?created=1` with three scroll-to chips (ScrollView ref + `onLayout` offsets); dismiss via param + local state. Keep `deriveSku` + AUD default. Tests: create submits the right body; hand-off replaces to `[id]` with the id; banner renders on the param and jumps. FULL suite + both tsc → 0. Commit `feat(mobile-admin): streamlined product create with hand-off to edit`.

---

### Task 6: `VariantRow` collapsible + `SectionDisclosure`

**Files:**
- Create: `apps/mobile-admin/components/products/SectionDisclosure.tsx`, `apps/mobile-admin/components/products/VariantRow.tsx`
- Modify: `apps/mobile-admin/app/(tabs)/products/[id].tsx` (map variants to `VariantRow`), `VariantEditor.tsx` (nest Shipping behind a disclosure)
- Test: `__tests__/variant-row.test.tsx`, `__tests__/section-disclosure.test.tsx`

**Interfaces:**
- Produces: `<SectionDisclosure title defaultOpen? children />` (reduced-motion-aware expand/collapse); `<VariantRow variant onUpdate />` (summary row: `variantLabel`, price·stock·sku caption, stock `StatusBadge`, `ChevronDown`; expands into `VariantEditor`). Single-variant products auto-expand.

- [ ] **Step 1–5** per spec Area 3: `SectionDisclosure` toggles content, honors `AccessibilityInfo.isReduceMotionEnabled` (instant when reduced). `VariantRow` wraps `VariantEditor`; the Shipping block (weight + 4 dims) moves behind a nested `SectionDisclosure` inside `VariantEditor`. Tests: row collapses/expands (assert body hidden when collapsed), single-variant auto-expands, `StatusBadge` tone maps stock. Keep the blur/NaN/rate-limit commit logic untouched. FULL suite + both tsc → 0. Commit `feat(mobile-admin): collapsible variant rows with nested shipping`.

---

### Task 7: Editorial rhythm pass

**Files:**
- Modify: `apps/mobile-admin/app/(tabs)/products/[id].tsx` (editorial header block, two movements, ghost cards, one-accent), migrate its inline inputs to `FieldInput`
- Modify: `OptionsEditor.tsx`, `VariantEditor.tsx` to use `FieldInput` where they still hand-roll inputs
- Test: extend `__tests__/product-detail-sections.test.tsx`

**Interfaces:** no new exports; visual/structural only.

- [ ] **Step 1–5** per spec Area 5: editorial header (thumb + serif title + `StatusBadge` on `surfaceAlt` band, hairline); group into Presentation/Commerce movements with `spacing.xxl` between; Details/Options/Categories become `Card variant="ghost"` + top `Hairline`, Photos + Variants stay bordered; moss only on header Save at rest (photo/option/category adds are ink, moss on press). Migrate inline inputs to `FieldInput`. Update the source-text section test for any renamed sections. FULL suite + both tsc → 0. Commit `feat(mobile-admin): editorial rhythm pass on the product editor`.

---

## Deploy / verification

- No backend change → nothing to promote; mobile-admin ships via EAS, not CI images.
- **CI:** repo is currently PUBLIC (user instruction, stays public until they say revert) so Actions minutes are unlimited. After the branch lands, watch the run; do NOT flip to private.
- **Device tap-through is the real acceptance test** — the human exercises add-option, the category sheet, streamlined create + hand-off, variant collapse, and confirms the layout. The controller can boot + screenshot the list but cannot tap.

## Self-review checklist (controller, before executing)

- Task 2 (`buildOptionMatrix`) is the highest-risk task — it MUST ship before Task 3 consumes it, and its test must prove no existing variant id is dropped when its tuple survives.
- `variants` is only ever populated by `buildOptionMatrix`. Grep the diff for any other `variants:` in a PATCH body.
- Every render test carries the lucide mock. Every sort/filter test uses an out-of-order fixture.
- Both tsc gates after every task. Never `npm install`.
