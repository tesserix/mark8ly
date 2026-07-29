import type { VariantEditField } from "@/lib/admin-api/variant-quick-edit";

/**
 * Every string the two quick-edit sheets and the Products menu read off the
 * FIELD, in one place — so the menu item, the picker's title, the sheet's
 * heading, the input's label and the failure notice can never drift into
 * describing four different actions.
 *
 * Keyed on `VariantEditField` rather than `string`, so a third quick-editable
 * field is a compile error here instead of a sheet titled `undefined`.
 */
export const FIELD_MENU_LABEL: Record<VariantEditField, string> = {
  price: "Edit price",
  inventory_quantity: "Adjust stock",
};

/** The sheet's own heading, and the picker's title above the variant rows. */
export const FIELD_TITLE = FIELD_MENU_LABEL;

/** The input's field label. */
export const FIELD_INPUT_LABEL: Record<VariantEditField, string> = {
  price: "Price",
  inventory_quantity: "Quantity",
};

/**
 * A lower-case verb phrase read after "Couldn't " on the screen's failure
 * notice — see `BusyIds.settleCallbacks`. Without one, a failed edit is
 * reported in the hand ALONE, which is the silence that surface exists to fix.
 */
export const FIELD_FAILURE_ACTION: Record<VariantEditField, string> = {
  price: "update this price",
  inventory_quantity: "update this stock level",
};
