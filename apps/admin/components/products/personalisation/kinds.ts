// Shared vocabulary for the personalisation editor.
//
// The kind decides which inputs a field has, so it is the first thing the
// merchant picks and the thing every other control keys off. Changing a
// kind after creation is deliberately not possible — it would orphan the
// uploads and invalidate the order snapshots that referenced the field —
// so the picker only ever appears when adding.

import type {
  PersonalisationField,
  PersonalisationKind,
} from "@/lib/api/marketplace-api";

export const KIND_CHOICES: ReadonlyArray<{
  value: PersonalisationKind;
  label: string;
  /** What the buyer sees. Written for a merchant, not a developer. */
  blurb: string;
}> = [
  {
    value: "image",
    label: "Image upload",
    blurb: "The buyer uploads a photo or artwork. For figurines, printed shirts, photo mugs.",
  },
  {
    value: "text",
    label: "Short text",
    blurb: "One line. A name to engrave, initials, a date.",
  },
  {
    value: "textarea",
    label: "Long text",
    blurb: "Several lines. A gift message, an inscription.",
  },
  {
    value: "select",
    label: "Choice",
    blurb: "The buyer picks one of your options. Each can add to the price.",
  },
  {
    value: "checkbox",
    label: "Add-on",
    blurb: "A yes/no extra, like gift wrap. Can add to the price.",
  },
];

export function kindLabel(kind: PersonalisationKind): string {
  return KIND_CHOICES.find((k) => k.value === kind)?.label ?? kind;
}

/**
 * One line describing what this field will ask of a buyer, built from
 * whichever columns the kind actually uses.
 *
 * This is the only place the editor restates the kind rules, and it reads
 * from the response rather than from form state — the API applies
 * defaults the merchant never typed (an image field gets max_images 1, a
 * short text gets 100 characters), and the summary should show what is
 * true rather than what was submitted.
 */
export function fieldSummary(
  field: PersonalisationField,
  currencyCode: string,
): string {
  switch (field.kind) {
    case "image": {
      const parts: string[] = [
        field.max_images && field.max_images > 1
          ? `up to ${field.max_images} images`
          : "one image",
      ];
      if (field.min_px) parts.push(`warns below ${field.min_px}px`);
      if (field.mockup_storage_key) parts.push("previewed on your mockup");
      return parts.join(" · ");
    }
    case "text":
    case "textarea":
      return `up to ${field.max_length ?? 0} characters`;
    case "select": {
      const options = field.options ?? [];
      if (options.length === 0) return "no choices yet";
      const paid = options.filter((o) => Number.parseFloat(o.price_delta) > 0).length;
      return paid === 0
        ? `${options.length} choices, all free`
        : `${options.length} choices, ${paid} priced`;
    }
    case "checkbox": {
      const delta = Number.parseFloat(field.price_delta ?? "0");
      return delta > 0 ? `adds ${currencyCode} ${field.price_delta}` : "free add-on";
    }
    default:
      return "";
  }
}

/** Derives a machine key from a label, matching the API's key format. */
export function keyFromLabel(label: string): string {
  return (
    label
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, "_")
      .replace(/^_+|_+$/g, "")
      .slice(0, 60)
      .replace(/_+$/, "") || "field"
  );
}

export const inputClass =
  "w-full rounded-md border border-[color:var(--ink-900)] border-opacity-20 bg-[color:var(--background-elevated,white)] px-3 py-2 text-sm text-[color:var(--ink-900)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]";
