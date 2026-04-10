"use client";

import { useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import type { SessionHeaders } from "@/lib/api/campaigns-api";
import { createSegment } from "@/lib/api/campaigns-api";

interface CreateSegmentFormProps {
  storeId: string;
  session: SessionHeaders;
}

type RuleType = "all" | "loyalty_tier" | "has_ordered" | "inactive_days";

interface RuleRow {
  type: RuleType;
  value: string;
}

const RULE_TYPE_OPTIONS: { value: RuleType; label: string }[] = [
  { value: "all", label: "All enrolled customers" },
  { value: "loyalty_tier", label: "By loyalty tier" },
  { value: "has_ordered", label: "Has placed an order" },
  { value: "inactive_days", label: "Inactive for N days" },
];

function ruleNeedsValue(type: RuleType): boolean {
  return type === "loyalty_tier" || type === "inactive_days";
}

function valuePlaceholder(type: RuleType): string {
  switch (type) {
    case "loyalty_tier":
      return "e.g. gold, silver";
    case "inactive_days":
      return "e.g. 90";
    default:
      return "";
  }
}

export function CreateSegmentForm({
  storeId,
  session,
}: CreateSegmentFormProps) {
  const router = useRouter();
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [rules, setRules] = useState<RuleRow[]>([{ type: "all", value: "" }]);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function addRule() {
    setRules((prev) => [...prev, { type: "all", value: "" }]);
  }

  function removeRule(index: number) {
    setRules((prev) => prev.filter((_, i) => i !== index));
  }

  function updateRule(index: number, field: "type" | "value", val: string) {
    setRules((prev) =>
      prev.map((r, i) =>
        i === index ? { ...r, [field]: val } : r,
      ),
    );
  }

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      if (!name.trim()) return;

      setSubmitting(true);
      setError(null);

      const rulesJson = JSON.stringify(
        rules.map((r) => ({
          type: r.type,
          ...(r.value ? { value: r.value } : {}),
        })),
      );

      try {
        const result = await createSegment(
          storeId,
          {
            name: name.trim(),
            description: description.trim() || undefined,
            rules: rulesJson,
          },
          session,
        );
        if (!result) {
          setError("Failed to create segment. Please check your rules and try again.");
          setSubmitting(false);
          return;
        }
        router.push("/marketing/segments");
        router.refresh();
      } catch {
        setError("An unexpected error occurred. Please try again.");
        setSubmitting(false);
      }
    },
    [name, description, rules, storeId, session, router],
  );

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      <div className="space-y-4">
        <label className="block">
          <span className="text-sm font-medium text-ink-700">
            Segment name
          </span>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g. Gold tier members"
            required
            className="mt-1 block w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 placeholder:text-ink-400 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
          />
        </label>

        <label className="block">
          <span className="text-sm font-medium text-ink-700">
            Description (optional)
          </span>
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={2}
            placeholder="Brief description of this audience segment"
            className="mt-1 block w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 placeholder:text-ink-400 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
          />
        </label>
      </div>

      <hr className="border-ink-200" />

      {/* Rules builder */}
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-medium text-ink-700">Rules</h3>
          <button
            type="button"
            onClick={addRule}
            className="text-sm font-medium text-moss-700 hover:text-moss-800"
          >
            + Add rule
          </button>
        </div>

        <p className="text-xs text-ink-400">
          Multiple rules are combined with AND logic (intersection).
        </p>

        {rules.map((rule, index) => (
          <div key={index} className="flex items-start gap-3">
            <div className="flex-1 space-y-2">
              <select
                value={rule.type}
                onChange={(e) =>
                  updateRule(index, "type", e.target.value)
                }
                className="block w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
              >
                {RULE_TYPE_OPTIONS.map((opt) => (
                  <option key={opt.value} value={opt.value}>
                    {opt.label}
                  </option>
                ))}
              </select>

              {ruleNeedsValue(rule.type) && (
                <input
                  type="text"
                  value={rule.value}
                  onChange={(e) =>
                    updateRule(index, "value", e.target.value)
                  }
                  placeholder={valuePlaceholder(rule.type)}
                  className="block w-full rounded-md border border-ink-200 bg-white px-3 py-2 text-sm text-ink-900 placeholder:text-ink-400 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
                />
              )}
            </div>

            {rules.length > 1 && (
              <button
                type="button"
                onClick={() => removeRule(index)}
                className="mt-2 text-xs text-signal-700 hover:text-signal-800"
                aria-label={`Remove rule ${index + 1}`}
              >
                Remove
              </button>
            )}
          </div>
        ))}
      </div>

      {error && (
        <p className="rounded-md bg-signal-50 px-3 py-2 text-sm text-signal-700">
          {error}
        </p>
      )}

      <div className="flex items-center justify-between pt-2">
        <button
          type="button"
          onClick={() => router.back()}
          className="inline-flex items-center gap-2 rounded-md border border-ink-200 bg-white px-4 py-2 text-sm font-medium text-ink-700 transition hover:bg-ink-50"
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={submitting || !name.trim()}
          className="inline-flex items-center gap-2 rounded-md bg-ink-900 px-4 py-2 text-sm font-medium text-paper-200 transition hover:bg-ink-800 disabled:opacity-50"
        >
          {submitting ? "Creating..." : "Create segment"}
        </button>
      </div>
    </form>
  );
}
