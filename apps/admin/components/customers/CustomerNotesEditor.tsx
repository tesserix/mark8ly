"use client";

import { useState, useTransition } from "react";

interface CustomerNotesEditorProps {
  customerId: string;
  initialNotes: string;
  updateAction: (customerId: string, notes: string) => Promise<{ ok: boolean; error?: string }>;
}

export function CustomerNotesEditor({
  customerId,
  initialNotes,
  updateAction,
}: CustomerNotesEditorProps) {
  const [notes, setNotes] = useState(initialNotes);
  const [saved, setSaved] = useState(initialNotes);
  const [isPending, startTransition] = useTransition();
  const [error, setError] = useState<string | null>(null);

  const isDirty = notes !== saved;

  function handleSave() {
    setError(null);
    startTransition(async () => {
      const result = await updateAction(customerId, notes);
      if (result.ok) {
        setSaved(notes);
      } else {
        setError(result.error ?? "Failed to save notes");
      }
    });
  }

  return (
    <section aria-labelledby="notes-heading" className="flex flex-col gap-4">
      <h2
        id="notes-heading"
        className="text-xs font-semibold uppercase tracking-[0.16em] text-foreground-tertiary"
      >
        Notes
      </h2>

      <textarea
        value={notes}
        onChange={(e) => setNotes(e.target.value)}
        rows={4}
        disabled={isPending}
        placeholder="Internal notes about this customer..."
        aria-label="Customer notes"
        className="w-full rounded-md border border-[color:var(--ink-900)]/20 bg-transparent px-3 py-2 text-sm text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none disabled:opacity-50"
      />

      <div className="flex items-center gap-3">
        <button
          type="button"
          onClick={handleSave}
          disabled={isPending || !isDirty}
          className="rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm font-medium text-[color:var(--primary-foreground)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-40"
        >
          {isPending ? "Saving..." : "Save notes"}
        </button>

        {error && (
          <p role="alert" className="text-sm text-[color:var(--danger)]">
            {error}
          </p>
        )}
      </div>
    </section>
  );
}
