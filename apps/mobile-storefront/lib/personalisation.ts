// Pure logic for the buyer's personalisation form (#969). No React, no
// fetch, no native module — the rules that decide whether someone can add
// to cart are testable without a simulator, which matters more here than
// usual because this app had no test runner at all before this file.
//
// A port of apps/storefront/lib/personalisation.ts. Kept semantically
// identical on purpose: web/mobile divergence in a shared concept is
// exactly what #1016 was.
//
// None of this is a security boundary. Checkout re-validates every answer
// server-side and re-prices every delta from the catalog (#967). What
// this governs is whether the buyer is allowed to *try*, and what they
// are told before they do.

import type {
  CheckoutLinePersonalisation,
  StorefrontPersonalisationField,
} from "@repo/mobile-shared/api/storefront-types";
import type { CartLinePersonalisation } from "@/lib/cart-store";

/**
 * What the buyer has entered so far, keyed by field id.
 *
 * An image field holds the ids of confirmed uploads. Everything else
 * holds a single scalar. Absent means untouched; present-but-empty means
 * they cleared it, and for a required field those are the same answer.
 */
export interface PersonalisationAnswer {
  text?: string;
  optionId?: string;
  checked?: boolean;
  uploadIds?: string[];
}

export type PersonalisationAnswers = Record<string, PersonalisationAnswer>;

/** A field that is not yet acceptable, and why — in the buyer's words. */
export interface PersonalisationProblem {
  fieldId: string;
  message: string;
}

/** Fields in the order the merchant arranged them. */
export function sortFields(
  fields: readonly StorefrontPersonalisationField[] | undefined,
): StorefrontPersonalisationField[] {
  return [...(fields ?? [])].sort(
    (a, b) => a.position - b.position || a.key.localeCompare(b.key),
  );
}

/**
 * Everything standing between the buyer and the Add to cart button.
 *
 * Returns problems rather than a boolean so the form can put each
 * message next to the control it belongs to.
 */
export function validateAnswers(
  fields: readonly StorefrontPersonalisationField[] | undefined,
  answers: PersonalisationAnswers,
): PersonalisationProblem[] {
  if (!fields || fields.length === 0) return [];

  const problems: PersonalisationProblem[] = [];
  for (const field of fields) {
    const answer = answers[field.id] ?? {};

    switch (field.kind) {
      case "image": {
        const n = answer.uploadIds?.length ?? 0;
        if (field.required && n === 0) {
          problems.push({
            fieldId: field.id,
            message: `Add a photo for “${field.label}”.`,
          });
        }
        const max = field.max_images ?? 1;
        if (n > max) {
          problems.push({
            fieldId: field.id,
            message:
              max === 1
                ? `Only one photo can be added for “${field.label}”.`
                : `At most ${max} photos can be added for “${field.label}”.`,
          });
        }
        break;
      }

      case "text":
      case "textarea": {
        const value = (answer.text ?? "").trim();
        if (field.required && value === "") {
          problems.push({
            fieldId: field.id,
            message: `“${field.label}” is required.`,
          });
        }
        if (field.max_length !== undefined && value.length > field.max_length) {
          problems.push({
            fieldId: field.id,
            message: `“${field.label}” must be ${field.max_length} characters or fewer.`,
          });
        }
        break;
      }

      case "select": {
        const chosen = answer.optionId;
        if (field.required && !chosen) {
          problems.push({
            fieldId: field.id,
            message: `Choose an option for “${field.label}”.`,
          });
        }
        // A choice the product does not offer is a stale form; the server
        // would reject it, and catching it here means the buyer is told
        // rather than bounced.
        if (chosen && !(field.options ?? []).some((o) => o.id === chosen)) {
          problems.push({
            fieldId: field.id,
            message: `That option is no longer available for “${field.label}”.`,
          });
        }
        break;
      }

      case "checkbox": {
        // A required checkbox means "you must tick this" — a consent or
        // an acknowledgement — so false is not an answer.
        if (field.required && answer.checked !== true) {
          problems.push({
            fieldId: field.id,
            message: `“${field.label}” must be ticked.`,
          });
        }
        break;
      }
    }
  }
  return problems;
}

/**
 * Converts the form's state into the shape the cart line carries.
 *
 * Only answers that mean something survive: an untouched optional field
 * contributes nothing, so two buyers who skipped it get the same line key
 * rather than two lines that differ by an empty string. Labels ride
 * along for the cart screen; they sit outside lineKey.
 */
export function toCartPersonalisation(
  fields: readonly StorefrontPersonalisationField[] | undefined,
  answers: PersonalisationAnswers,
): CartLinePersonalisation[] | undefined {
  if (!fields || fields.length === 0) return undefined;

  const out: CartLinePersonalisation[] = [];
  for (const field of fields) {
    const answer = answers[field.id];
    if (!answer) continue;

    switch (field.kind) {
      case "image":
        for (const uploadId of answer.uploadIds ?? []) {
          out.push({ fieldId: field.id, fieldLabel: field.label, uploadId });
        }
        break;
      case "text":
      case "textarea": {
        const value = (answer.text ?? "").trim();
        if (value !== "")
          out.push({ fieldId: field.id, fieldLabel: field.label, text: value });
        break;
      }
      case "select":
        if (answer.optionId) {
          out.push({
            fieldId: field.id,
            fieldLabel: field.label,
            optionId: answer.optionId,
            optionLabel: field.options?.find((o) => o.id === answer.optionId)
              ?.label,
          });
        }
        break;
      case "checkbox":
        // Only a TICKED box is an answer. Recording `false` would split
        // the cart line from an identical item someone left alone.
        if (answer.checked === true) {
          out.push({
            fieldId: field.id,
            fieldLabel: field.label,
            checked: true,
          });
        }
        break;
    }
  }
  return out.length > 0 ? out : undefined;
}

/**
 * Sum of what the buyer's choices add to one unit. Display only: checkout
 * recomputes this from the catalog (#967). It exists so the price on the
 * button matches the price on the receipt, not so it decides it.
 */
export function personalisationSurcharge(
  fields: readonly StorefrontPersonalisationField[] | undefined,
  answers: PersonalisationAnswers,
): number {
  if (!fields) return 0;
  let total = 0;
  for (const field of fields) {
    const answer = answers[field.id];
    if (!answer) continue;
    if (field.kind === "checkbox" && answer.checked === true) {
      total += Number.parseFloat(field.price_delta ?? "0") || 0;
    }
    if (field.kind === "select" && answer.optionId) {
      const picked = (field.options ?? []).find(
        (o) => o.id === answer.optionId,
      );
      total += Number.parseFloat(picked?.price_delta ?? "0") || 0;
    }
  }
  return total;
}

/**
 * Whether an image is too small to print well at the merchant's minimum.
 *
 * Compares the SHORT edge, because that is what limits print size: a
 * 4000x600 panorama is not a usable 1200px-square crop. False when the
 * merchant set no minimum or the image could not be measured — a warning
 * nobody can act on is worse than none.
 */
export function isBelowMinimumResolution(
  field: Pick<StorefrontPersonalisationField, "min_px">,
  dimensions: { width: number; height: number } | null | undefined,
): boolean {
  if (!field.min_px || !dimensions) return false;
  if (dimensions.width <= 0 || dimensions.height <= 0) return false;
  return Math.min(dimensions.width, dimensions.height) < field.min_px;
}

/**
 * A cart line's answers in the checkout request shape (#967). Strips the
 * display-only labels so the wire stays the contract. Undefined for a
 * line with nothing on it, so an ordinary product sends no empty array.
 */
export function toCheckoutPersonalisation(
  entries: readonly CartLinePersonalisation[] | undefined,
): CheckoutLinePersonalisation[] | undefined {
  if (!entries || entries.length === 0) return undefined;
  return entries.map((e) => ({
    field_id: e.fieldId,
    upload_id: e.uploadId,
    text: e.text,
    option_id: e.optionId,
    checked: e.checked,
  }));
}

/**
 * What the cart screen prints under a line: one "Label: value" per
 * answer, from the snapshots alone. A photo is named, not shown — its
 * preview URL is a ten-minute signed GET that would be dead by the time
 * the buyer scrolled to it, and the line already says which field it is.
 */
export function describeCartPersonalisation(
  entries: readonly CartLinePersonalisation[] | undefined,
): { label: string; value: string }[] {
  if (!entries) return [];
  const out: { label: string; value: string }[] = [];
  for (const e of entries) {
    const label = e.fieldLabel ?? "Personalisation";
    if (e.uploadId) out.push({ label, value: "Photo added" });
    else if (e.text !== undefined) out.push({ label, value: e.text });
    else if (e.optionId)
      out.push({ label, value: e.optionLabel ?? "Selected" });
    else if (e.checked) out.push({ label, value: "Yes" });
  }
  return out;
}
