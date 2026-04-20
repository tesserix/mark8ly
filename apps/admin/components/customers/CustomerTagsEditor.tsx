"use client";

import { useState, useTransition } from "react";

interface CustomerTagsEditorProps {
  customerId: string;
  initialTags: string[];
  updateAction: (customerId: string, tags: string[]) => Promise<{ ok: boolean; error?: string }>;
}

export function CustomerTagsEditor({
  customerId,
  initialTags,
  updateAction,
}: CustomerTagsEditorProps) {
  const [tags, setTags] = useState<string[]>(initialTags);
  const [input, setInput] = useState("");
  const [isPending, startTransition] = useTransition();
  const [error, setError] = useState<string | null>(null);

  function handleAdd() {
    const trimmed = input.trim().toLowerCase();
    if (!trimmed || tags.includes(trimmed)) {
      setInput("");
      return;
    }
    const next = [...tags, trimmed];
    setTags(next);
    setInput("");
    save(next);
  }

  function handleRemove(tag: string) {
    const next = tags.filter((t) => t !== tag);
    setTags(next);
    save(next);
  }

  function save(nextTags: string[]) {
    setError(null);
    startTransition(async () => {
      const result = await updateAction(customerId, nextTags);
      if (!result.ok) {
        setError(result.error ?? "Failed to update tags");
      }
    });
  }

  return (
    <section aria-labelledby="tags-heading" className="flex flex-col gap-4">
      <h2
        id="tags-heading"
        className="text-xs font-semibold uppercase tracking-[0.16em] text-foreground-tertiary"
      >
        Tags
      </h2>

      <div className="flex flex-wrap gap-2">
        {tags.map((tag) => (
          <span
            key={tag}
            className="flex items-center gap-1.5 rounded-sm bg-[color:var(--ink-900)]/[0.06] px-2 py-1 text-sm text-foreground"
          >
            {tag}
            <button
              type="button"
              onClick={() => handleRemove(tag)}
              disabled={isPending}
              className="text-foreground-tertiary transition-colors hover:text-foreground focus-visible:outline-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-30"
              aria-label={`Remove tag ${tag}`}
            >
              ×
            </button>
          </span>
        ))}
      </div>

      <div className="flex max-w-md gap-2">
        <input
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              handleAdd();
            }
          }}
          placeholder="Add a tag..."
          aria-label="Add a tag"
          disabled={isPending}
          className="flex-1 rounded-md border border-[color:var(--ink-900)]/20 bg-transparent px-3 py-2 text-sm text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none disabled:opacity-50"
        />
        <button
          type="button"
          onClick={handleAdd}
          disabled={isPending || !input.trim()}
          className="rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm font-medium text-[color:var(--primary-foreground)] transition-opacity hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:opacity-40"
        >
          Add
        </button>
      </div>

      {error && (
        <p role="alert" className="text-sm text-[color:var(--danger)]">
          {error}
        </p>
      )}
    </section>
  );
}
