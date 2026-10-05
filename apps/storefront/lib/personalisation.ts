// apps/storefront/lib/personalisation.ts
//
// Pure logic for the buyer's personalisation form (#965). No React, no
// fetch, no DOM — so the rules that decide whether someone can add to
// cart are testable without a browser, which matters more here than
// usual because the storefront has no e2e suite in CI (see #979).
//
// None of this is a security boundary. Checkout re-validates every answer
// server-side and re-prices every delta from the catalog (#967). What
// this governs is whether the buyer is allowed to *try*, and what they
// are told before they do.

import type {
  CartItemPersonalisation,
} from "./cart";
import type {
  StorefrontPersonalisationField,
} from "./api/marketplace-api";

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

/**
 * Everything standing between the buyer and the Add to cart button.
 *
 * Returns problems rather than a boolean so the form can put each
 * message next to the control it belongs to. A bare "something is
 * missing" on a product with six fields is not a usable error.
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
          problems.push({ fieldId: field.id, message: `Add an image for “${field.label}”.` });
        }
        // A cap of 1 is the common case and the message should not read
        // like a plural when it is not.
        const max = field.max_images ?? 1;
        if (n > max) {
          problems.push({
            fieldId: field.id,
            message:
              max === 1
                ? `Only one image can be added for “${field.label}”.`
                : `At most ${max} images can be added for “${field.label}”.`,
          });
        }
        break;
      }

      case "text":
      case "textarea": {
        const value = (answer.text ?? "").trim();
        if (field.required && value === "") {
          problems.push({ fieldId: field.id, message: `“${field.label}” is required.` });
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
          problems.push({ fieldId: field.id, message: `Choose an option for “${field.label}”.` });
        }
        // A choice the product does not offer is a stale form or a
        // tampered one; either way the server would reject it, and
        // catching it here means the buyer is told rather than bounced.
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
          problems.push({ fieldId: field.id, message: `“${field.label}” must be ticked.` });
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
 * rather than two lines that differ by an empty string.
 *
 * The field label — and for a select, the chosen option's label — are
 * snapshotted alongside (#966). The cart renders from localStorage with
 * no access to the product's field definitions, so without them it can
 * show a thumbnail and a value but not what either is FOR. They are
 * display-only and sit outside personalisationFingerprint, so they do
 * not change any line's identity.
 */
export function toCartPersonalisation(
  fields: readonly StorefrontPersonalisationField[] | undefined,
  answers: PersonalisationAnswers,
): CartItemPersonalisation[] | undefined {
  if (!fields || fields.length === 0) return undefined;

  const out: CartItemPersonalisation[] = [];
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
            // Resolved here, where the options are in hand. A bare
            // optionId on a cart line is a uuid the buyer cannot read.
            optionLabel: field.options?.find((o) => o.id === answer.optionId)?.label,
          });
        }
        break;
      case "checkbox":
        // Only a TICKED box is an answer. An unticked optional add-on is
        // the same as not having one, and recording `false` would split
        // the cart line from an identical item someone left alone.
        if (answer.checked === true)
          out.push({ fieldId: field.id, fieldLabel: field.label, checked: true });
        break;
    }
  }
  return out.length > 0 ? out : undefined;
}

/**
 * Sum of what the buyer's choices add to one unit, as a number.
 *
 * Display only. Checkout recomputes this from the catalog and never
 * trusts the client (#967) — this exists so the price on the button
 * matches the price on the receipt, not so it decides it.
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
      const picked = (field.options ?? []).find((o) => o.id === answer.optionId);
      total += Number.parseFloat(picked?.price_delta ?? "0") || 0;
    }
  }
  return total;
}

/**
 * Whether an image is too small to print well at the merchant's stated
 * minimum.
 *
 * Compares the SHORT edge, because that is what limits print size: a
 * 4000x600 panorama is not a usable 1200px-square crop. Returns false
 * when the merchant set no minimum or the browser could not measure the
 * image — an unmeasurable image is not evidence of a bad one, and a
 * warning nobody can act on is worse than none.
 */
export function isBelowMinimumResolution(
  field: Pick<StorefrontPersonalisationField, "min_px">,
  dimensions: { width: number; height: number } | null | undefined,
): boolean {
  if (!field.min_px || !dimensions) return false;
  if (dimensions.width <= 0 || dimensions.height <= 0) return false;
  return Math.min(dimensions.width, dimensions.height) < field.min_px;
}
