import { useMemo } from "react";
import { ActionSheet, type ActionSheetItem } from "@/components/ui";
import type { Product, ProductVariant } from "@repo/mobile-shared/api/types";
import type { VariantEditField } from "@/lib/admin-api/variant-quick-edit";
import { FIELD_TITLE } from "./variant-edit-copy";
import { sortVariants, variantLabel } from "./variant-identity";

export interface VariantPickTarget {
  product: Product;
  field: VariantEditField;
}

export interface VariantPickerSheetProps {
  /** The product whose variants are being chosen between, or null when the
   *  picker is closed. Null is also what CLEARS it — see `onDismiss`. */
  target: VariantPickTarget | null;
  onChoose: (variant: ProductVariant) => void;
  onDismiss: () => void;
}

/**
 * "Which one?" — the step between a per-PRODUCT gesture and a per-VARIANT
 * endpoint.
 *
 * `PATCH /products/:id/variants/:variantId` addresses one variant, and 8 of
 * the store's 12 active products have 2-5 of them. A quick edit therefore
 * cannot be a single tap on a multi-variant product without guessing which
 * variant the merchant meant, and guessing wrong writes a price to the wrong
 * SKU.
 *
 * A single-variant product never reaches this component: the caller opens the
 * numeric sheet directly, because that is the common case and it must not
 * cost an extra tap.
 *
 * Built on `ActionSheet` rather than as a fourth hand-rolled bottom sheet, and
 * that reuse is load-bearing rather than lazy: `ActionSheet` already owns the
 * snap-point arithmetic that scales with Dynamic Type (a four-item menu was
 * ~64pt short at accessibility sizes and lost its last row below the fold),
 * the `BottomSheetScrollView` body that keeps a long list reachable once that
 * height clamps under the notch, and the backdrop without which a tap meant
 * to cancel lands on the product row underneath. A picker for a 12-variant
 * product needs all three, and none of them are visible in a screenshot.
 *
 * Item keys are prefixed `variant-` so a variant id can never collide with a
 * menu action key, and so tests can tell the two sheets apart.
 */
export function VariantPickerSheet({ target, onChoose, onDismiss }: VariantPickerSheetProps) {
  const items = useMemo((): ActionSheetItem[] => {
    if (!target) return [];
    return sortVariants(target.product.variants).map((variant) => ({
      key: `variant-${variant.id}`,
      label: variantLabel(variant),
      onPress: () => onChoose(variant),
    }));
  }, [target, onChoose]);

  return (
    <ActionSheet
      // The ACTION, not the product: the rows below name the variants, and a
      // merchant two taps in needs reminding what they are choosing FOR.
      title={target ? FIELD_TITLE[target.field] : undefined}
      items={items}
      visible={target !== null}
      onDismiss={onDismiss}
    />
  );
}
