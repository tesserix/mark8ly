"use client";

// The Personalisation section of the product form (#962).
//
// Unlike the rest of the form, this section does NOT ride the product's
// Save: each change is its own request, applied immediately, the way
// Media works. A field carries money — a priced choice or a paid add-on —
// and holding those in an unsaved draft alongside the price fields would
// make it ambiguous which Save was the one that mattered.
//
// Actions are injectable so the tests drive the component without a
// server. Defaults are the real server actions.

import { useState, useTransition } from "react";

import type {
  CreatePersonalisationFieldInput,
  PersonalisationField,
} from "@/lib/api/marketplace-api";
import {
  addPersonalisationOptionAction as defaultAddOption,
  createPersonalisationFieldAction as defaultCreate,
  deletePersonalisationFieldAction as defaultDelete,
  deletePersonalisationOptionAction as defaultDeleteOption,
  updatePersonalisationFieldAction as defaultUpdate,
  type PersonalisationActionResult,
} from "@/app/(admin)/products/personalisation-actions";
import { AddPersonalisationField } from "./AddPersonalisationField";
import { PersonalisationFieldCard } from "./PersonalisationFieldCard";

export interface PersonalisationEditorDeps {
  createField?: (
    storeId: string,
    productId: string,
    input: CreatePersonalisationFieldInput,
  ) => Promise<PersonalisationActionResult>;
  updateField?: (
    storeId: string,
    productId: string,
    fieldId: string,
    input: Record<string, unknown>,
  ) => Promise<PersonalisationActionResult>;
  deleteField?: (
    storeId: string,
    productId: string,
    fieldId: string,
  ) => Promise<PersonalisationActionResult>;
  addOption?: (
    storeId: string,
    productId: string,
    fieldId: string,
    input: { value: string; label: string; price_delta?: string },
  ) => Promise<PersonalisationActionResult>;
  deleteOption?: (
    storeId: string,
    productId: string,
    fieldId: string,
    optionId: string,
  ) => Promise<PersonalisationActionResult>;
}

export interface PersonalisationEditorProps extends PersonalisationEditorDeps {
  storeId: string;
  productId: string;
  currencyCode: string;
  initialFields: PersonalisationField[];
}

export function PersonalisationEditor({
  storeId,
  productId,
  currencyCode,
  initialFields,
  createField = defaultCreate,
  updateField = defaultUpdate,
  deleteField = defaultDelete,
  addOption = defaultAddOption,
  deleteOption = defaultDeleteOption,
}: PersonalisationEditorProps) {
  const [fields, setFields] = useState<PersonalisationField[]>(initialFields);
  const [adding, setAdding] = useState(false);
  const [pending, startTransition] = useTransition();
  const [busyFieldId, setBusyFieldId] = useState<string | null>(null);
  // Errors are held per field (plus one slot for the add form) so a
  // failure on one row does not blank the message on another.
  const [errors, setErrors] = useState<Record<string, string>>({});

  function setError(scope: string, message?: string): void {
    setErrors((prev) => {
      const next = { ...prev };
      if (message) next[scope] = message;
      else delete next[scope];
      return next;
    });
  }

  /**
   * Applies a result: on success the returned field replaces the local
   * row, so the UI shows what the server actually stored rather than
   * what was submitted. A null field means it was deleted.
   */
  function apply(scope: string, fieldId: string | null, res: PersonalisationActionResult): void {
    if (!res.ok) {
      setError(scope, res.error.message);
      return;
    }
    setError(scope, undefined);
    if (res.field === null) {
      if (fieldId) setFields((prev) => prev.filter((f) => f.id !== fieldId));
      return;
    }
    const returned = res.field;
    setFields((prev) => {
      const exists = prev.some((f) => f.id === returned.id);
      const next = exists
        ? prev.map((f) => (f.id === returned.id ? returned : f))
        : [...prev, returned];
      return next.sort((a, b) => a.position - b.position || a.key.localeCompare(b.key));
    });
  }

  function run(
    scope: string,
    fieldId: string | null,
    work: () => Promise<PersonalisationActionResult>,
  ): void {
    setBusyFieldId(fieldId ?? "__add__");
    startTransition(async () => {
      const res = await work();
      apply(scope, fieldId, res);
      setBusyFieldId(null);
      if (scope === "__add__" && res.ok) setAdding(false);
    });
  }

  return (
    <div className="flex flex-col gap-4">
      {fields.length === 0 && !adding && (
        <p className="text-sm text-foreground-secondary">
          Nothing yet. Add a field to let buyers upload a photo, add a name to
          engrave, or pick a finish — the controls appear on the product page and
          their answers arrive with the order.
        </p>
      )}

      {fields.map((field) => (
        <PersonalisationFieldCard
          key={field.id}
          field={field}
          currencyCode={currencyCode}
          busy={pending && busyFieldId === field.id}
          error={errors[field.id]}
          onPatch={(patch) =>
            run(field.id, field.id, () => updateField(storeId, productId, field.id, patch))
          }
          onDelete={() =>
            run(field.id, field.id, () => deleteField(storeId, productId, field.id))
          }
          onAddOption={(input) =>
            run(field.id, field.id, () => addOption(storeId, productId, field.id, input))
          }
          onDeleteOption={(optionId) =>
            run(field.id, field.id, () =>
              deleteOption(storeId, productId, field.id, optionId),
            )
          }
        />
      ))}

      {adding ? (
        <AddPersonalisationField
          currencyCode={currencyCode}
          busy={pending && busyFieldId === "__add__"}
          error={errors["__add__"]}
          onCancel={() => {
            setAdding(false);
            setError("__add__", undefined);
          }}
          onSubmit={(input) =>
            run("__add__", null, () => createField(storeId, productId, input))
          }
        />
      ) : (
        <button
          type="button"
          onClick={() => setAdding(true)}
          className="self-start text-xs uppercase tracking-widest text-[var(--ink-500)] hover:text-[var(--moss-700)] focus:text-[var(--moss-700)] focus:outline-none"
        >
          + Add personalisation
        </button>
      )}
    </div>
  );
}
