"use client";

// The add form for one personalisation field.
//
// The kind is chosen first, and everything after it changes shape: this
// is the only form in the product page where that is true, which is why
// it is a disclosure rather than a row in a grid.

import { useState } from "react";

import type {
  CreatePersonalisationFieldInput,
  PersonalisationKind,
} from "@/lib/api/marketplace-api";
import { Field } from "@/components/products/form/Field";
import { inputClass, KIND_CHOICES, keyFromLabel } from "./kinds";

interface OptionDraft {
  value: string;
  label: string;
  price_delta: string;
}

export interface AddPersonalisationFieldProps {
  currencyCode: string;
  busy: boolean;
  /** Server-side validation message, already scoped to this form. */
  error?: string;
  onCancel: () => void;
  onSubmit: (input: CreatePersonalisationFieldInput) => void;
}

export function AddPersonalisationField({
  currencyCode,
  busy,
  error,
  onCancel,
  onSubmit,
}: AddPersonalisationFieldProps) {
  const [kind, setKind] = useState<PersonalisationKind>("image");
  const [label, setLabel] = useState("");
  // The key follows the label until the merchant edits it, then stops —
  // it cannot be changed after creation, so silently rewriting what they
  // typed would be worse than letting the two diverge.
  const [key, setKey] = useState("");
  const [keyTouched, setKeyTouched] = useState(false);
  const [required, setRequired] = useState(false);
  const [helpText, setHelpText] = useState("");
  const [maxImages, setMaxImages] = useState("1");
  const [minPx, setMinPx] = useState("");
  const [maxLength, setMaxLength] = useState("");
  const [priceDelta, setPriceDelta] = useState("");
  const [options, setOptions] = useState<OptionDraft[]>([
    { value: "", label: "", price_delta: "" },
  ]);

  const effectiveKey = keyTouched ? key : keyFromLabel(label);

  const filledOptions = options.filter((o) => o.label.trim().length > 0);
  // Mirrors the server rule rather than replacing it: a select is created
  // with its values or not at all, because a choice with nothing to
  // choose is a control a buyer cannot answer.
  const selectNeedsOptions = kind === "select" && filledOptions.length === 0;
  const canSubmit =
    !busy && label.trim().length > 0 && effectiveKey.length > 0 && !selectNeedsOptions;

  function submit(): void {
    const input: CreatePersonalisationFieldInput = {
      key: effectiveKey,
      label: label.trim(),
      kind,
      required,
    };
    if (helpText.trim()) input.help_text = helpText.trim();

    if (kind === "image") {
      const n = Number.parseInt(maxImages, 10);
      if (Number.isFinite(n) && n > 0) input.max_images = n;
      const px = Number.parseInt(minPx, 10);
      if (Number.isFinite(px) && px > 0) input.min_px = px;
    }
    if (kind === "text" || kind === "textarea") {
      const n = Number.parseInt(maxLength, 10);
      if (Number.isFinite(n) && n > 0) input.max_length = n;
    }
    if (kind === "checkbox" && priceDelta.trim()) {
      input.price_delta = priceDelta.trim();
    }
    if (kind === "select") {
      input.options = filledOptions.map((o, i) => ({
        value: o.value.trim() || keyFromLabel(o.label),
        label: o.label.trim(),
        price_delta: o.price_delta.trim() || "0",
        position: i * 10,
      }));
    }
    onSubmit(input);
  }

  return (
    <div className="rounded-md border border-[color:var(--ink-900)] border-opacity-15 bg-[color:var(--paper-100,transparent)] p-5">
      <fieldset disabled={busy} className="flex flex-col gap-5">
        <legend className="sr-only">Add a personalisation field</legend>

        <div className="flex flex-col gap-2">
          <span className="text-sm font-medium text-[color:var(--ink-900)]">
            What do you need from the buyer?
          </span>
          <div className="flex flex-col gap-2">
            {KIND_CHOICES.map((choice) => (
              <label
                key={choice.value}
                className="flex cursor-pointer items-start gap-3 rounded-md border border-[color:var(--ink-900)] border-opacity-10 px-3 py-2 hover:border-opacity-30"
              >
                <input
                  type="radio"
                  name="personalisation-kind"
                  value={choice.value}
                  checked={kind === choice.value}
                  onChange={() => setKind(choice.value)}
                  className="mt-1"
                />
                <span className="min-w-0">
                  <span className="block text-sm text-[color:var(--ink-900)]">
                    {choice.label}
                  </span>
                  <span className="block text-xs text-foreground-tertiary">
                    {choice.blurb}
                  </span>
                </span>
              </label>
            ))}
          </div>
        </div>

        <Field
          label="Label the buyer sees"
          helper="Written as an instruction — “Upload your photo”, not “Photo”."
        >
          <input
            type="text"
            value={label}
            onChange={(e) => setLabel(e.target.value)}
            placeholder="Upload your photo"
            className={inputClass}
          />
        </Field>

        <Field
          label="Reference"
          helper="Used in your orders and exports. This cannot be changed later."
        >
          <input
            type="text"
            value={effectiveKey}
            onChange={(e) => {
              setKeyTouched(true);
              setKey(e.target.value);
            }}
            className={`${inputClass} font-mono`}
          />
        </Field>

        <Field label="Help text" helper="Optional. Shown under the control.">
          <input
            type="text"
            value={helpText}
            onChange={(e) => setHelpText(e.target.value)}
            placeholder="A clear, well-lit photo works best."
            className={inputClass}
          />
        </Field>

        {kind === "image" && (
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="How many images" helper="Between 1 and 10.">
              <input
                type="number"
                min={1}
                max={10}
                value={maxImages}
                onChange={(e) => setMaxImages(e.target.value)}
                className={inputClass}
              />
            </Field>
            <Field
              label="Warn below (pixels)"
              helper="Optional. Tells the buyer their photo may print soft — cheaper than a refund."
            >
              <input
                type="number"
                min={1}
                value={minPx}
                onChange={(e) => setMinPx(e.target.value)}
                placeholder="1200"
                className={inputClass}
              />
            </Field>
          </div>
        )}

        {(kind === "text" || kind === "textarea") && (
          <Field
            label="Character limit"
            helper={
              kind === "text"
                ? "Leave blank for 100, the default for one line."
                : "Leave blank for 500, the default for a message."
            }
          >
            <input
              type="number"
              min={1}
              value={maxLength}
              onChange={(e) => setMaxLength(e.target.value)}
              className={inputClass}
            />
          </Field>
        )}

        {kind === "checkbox" && (
          <Field
            label={`Adds to the price (${currencyCode})`}
            helper="Leave blank for a free add-on."
          >
            <input
              type="text"
              inputMode="decimal"
              value={priceDelta}
              onChange={(e) => setPriceDelta(e.target.value)}
              placeholder="4.00"
              className={inputClass}
            />
          </Field>
        )}

        {kind === "select" && (
          <div className="flex flex-col gap-3">
            <span className="text-sm font-medium text-[color:var(--ink-900)]">
              Choices
            </span>
            {options.map((opt, i) => (
              <div key={i} className="flex flex-wrap items-end gap-2">
                <div className="min-w-[12rem] flex-1">
                  <Field label={i === 0 ? "Choice" : ""}>
                    <input
                      type="text"
                      value={opt.label}
                      onChange={(e) =>
                        setOptions((prev) =>
                          prev.map((o, j) =>
                            j === i ? { ...o, label: e.target.value } : o,
                          ),
                        )
                      }
                      placeholder="Gloss finish"
                      className={inputClass}
                    />
                  </Field>
                </div>
                <div className="w-32">
                  <Field label={i === 0 ? `Adds (${currencyCode})` : ""}>
                    <input
                      type="text"
                      inputMode="decimal"
                      value={opt.price_delta}
                      onChange={(e) =>
                        setOptions((prev) =>
                          prev.map((o, j) =>
                            j === i ? { ...o, price_delta: e.target.value } : o,
                          ),
                        )
                      }
                      placeholder="0.00"
                      className={inputClass}
                    />
                  </Field>
                </div>
                {options.length > 1 && (
                  <button
                    type="button"
                    onClick={() => setOptions((prev) => prev.filter((_, j) => j !== i))}
                    className="pb-2 text-xs uppercase tracking-widest text-[var(--ink-500)] hover:text-[var(--signal)]"
                  >
                    Remove
                  </button>
                )}
              </div>
            ))}
            <button
              type="button"
              onClick={() =>
                setOptions((prev) => [...prev, { value: "", label: "", price_delta: "" }])
              }
              className="self-start text-xs uppercase tracking-widest text-[var(--ink-500)] hover:text-[var(--moss-700)]"
            >
              + Add choice
            </button>
            {selectNeedsOptions && (
              <p className="text-xs text-foreground-tertiary">
                A choice needs at least one option before a buyer can answer it.
              </p>
            )}
          </div>
        )}

        <label className="flex items-center gap-2 text-sm text-[color:var(--ink-900)]">
          <input
            type="checkbox"
            checked={required}
            onChange={(e) => setRequired(e.target.checked)}
          />
          The buyer must fill this in before adding to their cart
        </label>

        {error && (
          <p role="alert" className="text-xs text-[color:var(--signal,#C23B22)]">
            {error}
          </p>
        )}

        <div className="flex items-center gap-4">
          <button
            type="button"
            onClick={submit}
            disabled={!canSubmit}
            className="rounded-md bg-[color:var(--moss-700)] px-4 py-2 text-sm text-white disabled:opacity-40"
          >
            {busy ? "Adding…" : "Add field"}
          </button>
          <button
            type="button"
            onClick={onCancel}
            className="text-xs uppercase tracking-widest text-[var(--ink-500)] hover:text-[var(--ink-900)]"
          >
            Cancel
          </button>
        </div>
      </fieldset>
    </div>
  );
}
