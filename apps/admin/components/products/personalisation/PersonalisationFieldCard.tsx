"use client";

// One personalisation field, as the merchant sees it after creating it.
//
// Reads from the API response rather than from any local draft: the API
// fills in defaults nobody typed (an image field gets one image, a short
// text gets 100 characters), so showing submitted values would show
// blanks where the buyer will see a real limit.

import { useState } from "react";
import { Trash2 } from "lucide-react";

import type { PersonalisationField } from "@/lib/api/marketplace-api";
import { Field } from "@/components/products/form/Field";
import { fieldSummary, inputClass, keyFromLabel, kindLabel } from "./kinds";

import {
  MockupConfig,
  type MockupMedia,
  type PrintAreaValue,
} from "./MockupConfig";

export interface PersonalisationFieldCardProps {
  field: PersonalisationField;
  currencyCode: string;
  /**
   * The product's own images, offered as the mockup for the 2D preview
   * (#966). Empty simply hides the control.
   */
  media?: readonly MockupMedia[];
  /** Saves the mockup pair. Separate from onPatch: it writes two
   *  nullable columns together and is its own action, not a field edit. */
  onSaveMockup?: (patch: {
    mockup_storage_key: string | null;
    print_area: PrintAreaValue | null;
  }) => void;
  busy: boolean;
  error?: string;
  onPatch: (patch: {
    label?: string;
    required?: boolean;
    help_text?: string;
    max_length?: number;
    max_images?: number;
    min_px?: number;
    price_delta?: string;
  }) => void;
  onDelete: () => void;
  onAddOption: (input: { value: string; label: string; price_delta?: string }) => void;
  onDeleteOption: (optionId: string) => void;
}

export function PersonalisationFieldCard({
  field,
  media,
  onSaveMockup,
  currencyCode,
  busy,
  error,
  onPatch,
  onDelete,
  onAddOption,
  onDeleteOption,
}: PersonalisationFieldCardProps) {
  const [editing, setEditing] = useState(false);
  const [label, setLabel] = useState(field.label);
  const [helpText, setHelpText] = useState(field.help_text ?? "");
  const [numeric, setNumeric] = useState(
    String(field.max_length ?? field.max_images ?? ""),
  );
  const [minPx, setMinPx] = useState(String(field.min_px ?? ""));
  const [priceDelta, setPriceDelta] = useState(field.price_delta ?? "");
  const [newOption, setNewOption] = useState({ label: "", price_delta: "" });

  const options = field.options ?? [];

  function saveEdits(): void {
    const patch: Parameters<typeof onPatch>[0] = {};
    if (label.trim() && label.trim() !== field.label) patch.label = label.trim();
    if (helpText !== (field.help_text ?? "")) patch.help_text = helpText;

    const n = Number.parseInt(numeric, 10);
    if (Number.isFinite(n) && n > 0) {
      if (field.kind === "image" && n !== field.max_images) patch.max_images = n;
      if (
        (field.kind === "text" || field.kind === "textarea") &&
        n !== field.max_length
      ) {
        patch.max_length = n;
      }
    }
    if (field.kind === "image") {
      const px = Number.parseInt(minPx, 10);
      if (Number.isFinite(px) && px > 0 && px !== field.min_px) patch.min_px = px;
    }
    // Sent whenever it differs, including when it becomes "0": making a
    // paid add-on free is a change a zero-check would swallow.
    if (field.kind === "checkbox" && priceDelta.trim() !== (field.price_delta ?? "")) {
      patch.price_delta = priceDelta.trim() || "0";
    }

    if (Object.keys(patch).length > 0) onPatch(patch);
    setEditing(false);
  }

  return (
    <div className="rounded-md border border-[color:var(--ink-900)] border-opacity-15 p-4">
      <div className="flex flex-wrap items-start justify-between gap-x-6 gap-y-2">
        <div className="min-w-0">
          <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
            <span className="text-sm text-[color:var(--ink-900)]">{field.label}</span>
            <span className="text-xs uppercase tracking-widest text-[var(--ink-500)]">
              {kindLabel(field.kind)}
            </span>
            {field.required && (
              <span className="text-xs uppercase tracking-widest text-[var(--moss-700)]">
                Required
              </span>
            )}
          </div>
          <p className="mt-1 text-xs text-foreground-tertiary">
            <span className="font-mono">{field.key}</span>
            {" · "}
            {fieldSummary(field, currencyCode)}
          </p>
          {field.help_text && !editing && (
            <p className="mt-1 text-xs text-foreground-secondary">{field.help_text}</p>
          )}
        </div>

        <div className="flex items-center gap-3">
          <label className="flex items-center gap-2 text-xs text-foreground-secondary">
            <input
              type="checkbox"
              checked={field.required}
              disabled={busy}
              onChange={(e) => onPatch({ required: e.target.checked })}
            />
            Required
          </label>
          <button
            type="button"
            onClick={() => setEditing((v) => !v)}
            disabled={busy}
            className="text-xs uppercase tracking-widest text-[var(--ink-500)] hover:text-[var(--ink-900)] disabled:opacity-40"
          >
            {editing ? "Close" : "Edit"}
          </button>
          <button
            type="button"
            onClick={onDelete}
            disabled={busy}
            aria-label={`Delete ${field.label}`}
            className="text-[var(--ink-500)] hover:text-[color:var(--signal,#C23B22)] disabled:opacity-40"
          >
            <Trash2 aria-hidden size={16} />
          </button>
        </div>
      </div>

      {editing && (
        <fieldset disabled={busy} className="mt-4 flex flex-col gap-4 border-t border-border-subtle pt-4">
          <Field label="Label the buyer sees">
            <input
              type="text"
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              className={inputClass}
            />
          </Field>
          <Field label="Help text">
            <input
              type="text"
              value={helpText}
              onChange={(e) => setHelpText(e.target.value)}
              className={inputClass}
            />
          </Field>

          {field.kind === "image" && (
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="How many images">
                <input
                  type="number"
                  min={1}
                  max={10}
                  value={numeric}
                  onChange={(e) => setNumeric(e.target.value)}
                  className={inputClass}
                />
              </Field>
              <Field label="Warn below (pixels)">
                <input
                  type="number"
                  min={1}
                  value={minPx}
                  onChange={(e) => setMinPx(e.target.value)}
                  className={inputClass}
                />
              </Field>
            </div>
          )}

          {field.kind === "image" && onSaveMockup ? (
            <div className="rounded-md border border-[var(--ink-100)] p-3">
              <MockupConfig
                media={media ?? []}
                mockupStorageKey={field.mockup_storage_key}
                printArea={field.print_area}
                busy={busy}
                onSave={onSaveMockup}
              />
            </div>
          ) : null}

          {(field.kind === "text" || field.kind === "textarea") && (
            <Field label="Character limit">
              <input
                type="number"
                min={1}
                value={numeric}
                onChange={(e) => setNumeric(e.target.value)}
                className={inputClass}
              />
            </Field>
          )}

          {field.kind === "checkbox" && (
            <Field
              label={`Adds to the price (${currencyCode})`}
              helper="Set to 0 to make it free."
            >
              <input
                type="text"
                inputMode="decimal"
                value={priceDelta}
                onChange={(e) => setPriceDelta(e.target.value)}
                className={inputClass}
              />
            </Field>
          )}

          <div>
            <button
              type="button"
              onClick={saveEdits}
              className="rounded-md bg-[color:var(--moss-700)] px-4 py-2 text-sm text-white disabled:opacity-40"
            >
              {busy ? "Saving…" : "Save"}
            </button>
          </div>
        </fieldset>
      )}

      {field.kind === "select" && (
        <div className="mt-4 flex flex-col gap-2 border-t border-border-subtle pt-4">
          {options.map((opt) => {
            const delta = Number.parseFloat(opt.price_delta);
            return (
              <div key={opt.id} className="flex items-center justify-between gap-4">
                <span className="text-sm text-[color:var(--ink-900)]">
                  {opt.label}
                  {delta > 0 && (
                    <span className="ml-2 text-xs text-foreground-secondary">
                      +{currencyCode} {opt.price_delta}
                    </span>
                  )}
                </span>
                <button
                  type="button"
                  onClick={() => onDeleteOption(opt.id)}
                  // The last choice cannot be removed: an active product
                  // with an empty choice would reject every checkout of
                  // itself. The server refuses it too; this stops the
                  // merchant finding out by being refused.
                  disabled={busy || options.length <= 1}
                  title={
                    options.length <= 1
                      ? "A choice must keep at least one option. Delete the field instead."
                      : undefined
                  }
                  className="text-xs uppercase tracking-widest text-[var(--ink-500)] hover:text-[color:var(--signal)] disabled:opacity-30"
                >
                  Remove
                </button>
              </div>
            );
          })}

          <div className="mt-2 flex flex-wrap items-end gap-2">
            <div className="min-w-[10rem] flex-1">
              <Field label="Add a choice">
                <input
                  type="text"
                  value={newOption.label}
                  onChange={(e) =>
                    setNewOption((p) => ({ ...p, label: e.target.value }))
                  }
                  placeholder="Matte finish"
                  className={inputClass}
                />
              </Field>
            </div>
            <div className="w-28">
              <Field label={`Adds (${currencyCode})`}>
                <input
                  type="text"
                  inputMode="decimal"
                  value={newOption.price_delta}
                  onChange={(e) =>
                    setNewOption((p) => ({ ...p, price_delta: e.target.value }))
                  }
                  placeholder="0.00"
                  className={inputClass}
                />
              </Field>
            </div>
            <button
              type="button"
              disabled={busy || newOption.label.trim().length === 0}
              onClick={() => {
                onAddOption({
                  value: keyFromLabel(newOption.label),
                  label: newOption.label.trim(),
                  price_delta: newOption.price_delta.trim() || "0",
                });
                setNewOption({ label: "", price_delta: "" });
              }}
              className="mb-[2px] rounded-md border border-[color:var(--ink-900)] border-opacity-20 px-3 py-2 text-sm text-[color:var(--ink-900)] disabled:opacity-40"
            >
              Add
            </button>
          </div>
        </div>
      )}

      {error && (
        <p role="alert" className="mt-3 text-xs text-[color:var(--signal,#C23B22)]">
          {error}
        </p>
      )}
    </div>
  );
}
