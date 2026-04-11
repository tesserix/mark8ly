"use client";

import { useActionState, useState } from "react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@tesserix/web";
import { createTicketAction } from "@/app/(admin)/support/actions";

type Priority = "low" | "medium" | "high";

export function TicketCreateForm() {
  const [state, formAction, isPending] = useActionState(
    createTicketAction,
    null,
  );
  const [editing, setEditing] = useState(false);
  const [priority, setPriority] = useState<Priority>("medium");

  const showError = state?.error && !editing;

  function handleInput() {
    if (!editing) setEditing(true);
  }

  return (
    <form action={formAction} className="space-y-6">
      {showError && (
        <p
          role="alert"
          className="rounded-md border border-[color:var(--signal)]/20 bg-[color:var(--signal)]/5 px-4 py-3 text-sm text-[color:var(--signal)]"
        >
          {state.error}
        </p>
      )}

      <div className="space-y-2">
        <label
          htmlFor="subject"
          className="block text-sm font-medium text-foreground"
        >
          Subject <span className="text-[color:var(--signal)]">*</span>
        </label>
        <input
          id="subject"
          name="subject"
          type="text"
          required
          className="h-11 w-full rounded-md border border-border bg-background px-4 text-sm text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
          placeholder="Brief summary of your issue"
          onInput={handleInput}
        />
      </div>

      <div className="space-y-2">
        <label
          htmlFor="description"
          className="block text-sm font-medium text-foreground"
        >
          Description <span className="text-[color:var(--signal)]">*</span>
        </label>
        <textarea
          id="description"
          name="description"
          required
          minLength={20}
          rows={6}
          className="w-full rounded-md border border-border bg-background px-4 py-3 text-sm text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)]"
          placeholder="Describe your issue in detail (minimum 20 characters)"
          onInput={handleInput}
        />
      </div>

      <div className="space-y-2">
        <label
          htmlFor="priority"
          className="block text-sm font-medium text-foreground"
        >
          Priority
        </label>
        {/* Hidden input carries the value into the server action form data
            since @tesserix/web Select is controlled, not a native form
            control that posts itself. */}
        <input type="hidden" name="priority" value={priority} />
        <Select
          value={priority}
          onValueChange={(value) => {
            setPriority(value as Priority);
            handleInput();
          }}
        >
          <SelectTrigger id="priority" className="w-full sm:max-w-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="low">Low</SelectItem>
            <SelectItem value="medium">Medium</SelectItem>
            <SelectItem value="high">High</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <div className="flex items-center gap-4 pt-2">
        <button
          type="submit"
          disabled={isPending}
          className="inline-flex h-11 items-center justify-center rounded-md bg-primary px-6 text-sm font-medium text-primary-foreground hover:bg-primary-hover disabled:opacity-50"
        >
          {isPending ? "Creating..." : "Create ticket"}
        </button>
        <a
          href="/support/tickets"
          className="text-sm text-foreground-secondary hover:text-foreground"
        >
          Cancel
        </a>
      </div>
    </form>
  );
}
