"use client";

import { useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@tesserix/web";
import type {
  AdminSegment,
  SessionHeaders,
} from "@/lib/api/campaigns-api";
import { createSegment, updateSegment } from "@/lib/api/campaigns-api";
import type { LoyaltyTier } from "@/lib/api/loyalty-api";

type Mode = "create" | "edit";

interface SegmentFormProps {
  storeId: string;
  session: SessionHeaders;
  tiers: LoyaltyTier[];
  initialSegment?: AdminSegment;
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

function parseInitialRules(rulesJson: string | undefined): RuleRow[] {
  if (!rulesJson) return [{ type: "all", value: "" }];
  try {
    const parsed = JSON.parse(rulesJson) as { type: string; value?: string }[];
    if (!Array.isArray(parsed) || parsed.length === 0) {
      return [{ type: "all", value: "" }];
    }
    return parsed.map((r) => ({
      type: (["all", "loyalty_tier", "has_ordered", "inactive_days"].includes(
        r.type,
      )
        ? r.type
        : "all") as RuleType,
      value: r.value ?? "",
    }));
  } catch {
    return [{ type: "all", value: "" }];
  }
}

export function SegmentForm({
  storeId,
  session,
  tiers,
  initialSegment,
}: SegmentFormProps) {
  const router = useRouter();
  const mode: Mode = initialSegment ? "edit" : "create";

  const [name, setName] = useState(initialSegment?.name ?? "");
  const [description, setDescription] = useState(
    initialSegment?.description ?? "",
  );
  const [rules, setRules] = useState<RuleRow[]>(() =>
    parseInitialRules(initialSegment?.rules),
  );
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const hasTiers = tiers.length > 0;

  function addRule() {
    setRules((prev) => [...prev, { type: "all", value: "" }]);
  }

  function removeRule(index: number) {
    setRules((prev) => prev.filter((_, i) => i !== index));
  }

  function updateRule(index: number, field: "type" | "value", val: string) {
    setRules((prev) =>
      prev.map((r, i) => (i === index ? { ...r, [field]: val } : r)),
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

      const body = {
        name: name.trim(),
        description: description.trim() || undefined,
        rules: rulesJson,
      };

      try {
        const result =
          mode === "edit" && initialSegment
            ? await updateSegment(storeId, initialSegment.id, body, session)
            : await createSegment(storeId, body, session);

        if (!result) {
          setError(
            mode === "edit"
              ? "Failed to update segment. Please check your rules and try again."
              : "Failed to create segment. Please check your rules and try again.",
          );
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
    [mode, name, description, rules, storeId, session, router, initialSegment],
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
            className="mt-1 block w-full rounded-md border border-ink-200 bg-background-elevated px-3 py-2 text-sm text-ink-900 placeholder:text-ink-500 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
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
            className="mt-1 block w-full rounded-md border border-ink-200 bg-background-elevated px-3 py-2 text-sm text-ink-900 placeholder:text-ink-500 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
          />
        </label>
      </div>

      <hr className="border-ink-200" />

      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-medium text-ink-700">Rules</h3>
          <button
            type="button"
            onClick={addRule}
            className="text-sm font-medium text-moss-700 hover:text-moss-700/90"
          >
            + Add rule
          </button>
        </div>

        <p className="text-xs text-ink-500">
          Multiple rules are combined with AND logic (intersection).
        </p>

        {rules.map((rule, index) => (
          <div key={index} className="flex items-start gap-3">
            <div className="flex-1 space-y-2">
              <Select
                value={rule.type}
                onValueChange={(value) => updateRule(index, "type", value)}
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {RULE_TYPE_OPTIONS.map((opt) => (
                    <SelectItem key={opt.value} value={opt.value}>
                      {opt.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>

              {rule.type === "loyalty_tier" && hasTiers && (
                <Select
                  value={rule.value}
                  onValueChange={(value) => updateRule(index, "value", value)}
                >
                  <SelectTrigger className="w-full">
                    <SelectValue placeholder="Select a tier" />
                  </SelectTrigger>
                  <SelectContent>
                    {tiers.map((tier) => (
                      <SelectItem key={tier.name} value={tier.name}>
                        {tier.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}

              {rule.type === "loyalty_tier" && !hasTiers && (
                <div className="rounded-md border border-dashed border-ink-200 px-3 py-2 text-xs text-ink-500">
                  No loyalty tiers configured.{" "}
                  <Link
                    href="/marketing/loyalty"
                    className="font-medium text-moss-700 underline-offset-2 hover:underline"
                  >
                    Set up your loyalty program
                  </Link>{" "}
                  first.
                </div>
              )}

              {rule.type === "inactive_days" && (
                <input
                  type="number"
                  min={1}
                  value={rule.value}
                  onChange={(e) => updateRule(index, "value", e.target.value)}
                  placeholder="e.g. 90"
                  className="block w-full rounded-md border border-ink-200 bg-background-elevated px-3 py-2 text-sm text-ink-900 placeholder:text-ink-500 focus:border-moss-700 focus:outline-none focus:ring-1 focus:ring-moss-700"
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
          className="inline-flex items-center gap-2 rounded-md border border-ink-200 bg-background-elevated px-4 py-2 text-sm font-medium text-ink-700 transition hover:bg-ink-50"
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={submitting || !name.trim()}
          className="inline-flex items-center gap-2 rounded-md bg-ink-900 px-4 py-2 text-sm font-medium text-paper-200 transition hover:bg-ink-800 disabled:opacity-50"
        >
          {submitting
            ? mode === "edit"
              ? "Saving..."
              : "Creating..."
            : mode === "edit"
              ? "Save changes"
              : "Create segment"}
        </button>
      </div>
    </form>
  );
}
