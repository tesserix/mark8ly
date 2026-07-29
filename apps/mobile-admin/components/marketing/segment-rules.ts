/**
 * The segment rules model, as the backend actually defines it.
 *
 * `SegmentRule` (services/marketplace-api/internal/campaign/models.go:116-133)
 * is three STRING fields — `type`, `field`, `value` — and the engine
 * (segment_engine.go:47-66) switches on exactly four `type` values and reads
 * `Value` for the two that carry one. There is no `operator`, no
 * `total_spent`, and no numeric anything: `json.Unmarshal` into a `string`
 * field rejects `"value": 100` outright, which is why the mobile form's old
 * placeholder was a 400 for any merchant who copied it.
 *
 * Everything here is pure so the wire shape can be asserted directly — the
 * defect being fixed is a client that invented a schema and never noticed.
 */

/** The four types `SegmentEngine.ResolveEmails` will accept. Anything else is
 *  `unknown rule type` at campaign-send time. */
export const RULE_TYPES = ["all", "has_ordered", "loyalty_tier", "inactive_days"] as const;

export type SegmentRuleType = (typeof RULE_TYPES)[number];

export const RULE_TYPE_LABEL: Record<SegmentRuleType, string> = {
  all: "Everyone in your store",
  has_ordered: "Has placed an order",
  loyalty_tier: "In a loyalty tier",
  inactive_days: "Hasn't ordered in a while",
};

/** One line under the chosen type, so the merchant knows who it selects
 *  without leaving for the web dashboard to find out. */
export const RULE_TYPE_HINT: Record<SegmentRuleType, string> = {
  all: "Every customer enrolled in your store who accepts marketing.",
  has_ordered: "Customers with at least one order.",
  loyalty_tier: "Customers currently sitting in one loyalty tier.",
  inactive_days: "Customers with no order in the last N days.",
};

/**
 * Whether a type carries a `value` at all.
 *
 * `all` and `has_ordered` are whole-store predicates — the engine never reads
 * `Value` for them — so the form must not show a dead field for them, and the
 * serialiser must not smuggle a leftover one onto the wire.
 */
export function ruleTakesValue(type: SegmentRuleType): boolean {
  return type === "loyalty_tier" || type === "inactive_days";
}

function isRuleType(value: unknown): value is SegmentRuleType {
  return typeof value === "string" && (RULE_TYPES as readonly string[]).includes(value);
}

/** A rule this screen can show and edit. `field` is carried through rather
 *  than edited: the engine ignores it, but rewriting a stored qualifier the
 *  merchant cannot see would be exactly the silent change this file exists to
 *  avoid. */
export interface KnownRuleRow {
  kind: "known";
  id: string;
  type: SegmentRuleType;
  field: string;
  value: string;
}

/**
 * A rule the builder cannot represent, kept VERBATIM.
 *
 * The deliberate decision (see the task brief): an unrepresentable rule is
 * neither dropped nor coerced to `all`. Both of those silently change who the
 * segment targets. It is shown as a read-only row the merchant can see and
 * delete, and `raw` is re-emitted byte-for-byte on save, so editing a
 * segment's NAME can never rewrite its audience.
 */
export interface UnsupportedRuleRow {
  kind: "unsupported";
  id: string;
  raw: unknown;
  /** The rule as JSON, for the read-only row to display. */
  summary: string;
}

export type RuleRow = KnownRuleRow | UnsupportedRuleRow;

/**
 * The whole rules field as the form holds it.
 *
 * `opaque` is the escape hatch for stored rules that are not even a JSON
 * ARRAY (the engine's `json.Unmarshal` into `[]SegmentRule` would fail on
 * them too). There is no honest way to render those as rows, so the form
 * shows them read-only and re-sends them untouched unless the merchant
 * explicitly replaces them.
 */
export type RulesDraft =
  | { mode: "rows"; rows: RuleRow[] }
  | { mode: "opaque"; raw: string };

/** The wire shape, exactly: three strings, no more, no less. */
export interface WireSegmentRule {
  type: string;
  field: string;
  value: string;
}

export function newRuleRow(id: string, type: SegmentRuleType = "all"): KnownRuleRow {
  return { kind: "known", id, type, field: "", value: "" };
}

/**
 * A stored `value` as the wire requires it: a STRING.
 *
 * Numbers are coerced rather than rejected. A `{"type":"inactive_days",
 * "value":90}` cannot round-trip through the Go model, so a stored one is
 * already broken; turning it into `"90"` repairs it in the one direction that
 * is unambiguous, and the merchant sees the repaired value in the field
 * before they save.
 */
function stringField(value: unknown): string {
  if (typeof value === "string") return value;
  if (typeof value === "number" && Number.isFinite(value)) return String(value);
  return "";
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/**
 * Turn the stored rules string into editable rows.
 *
 * An absent or empty rules string opens on a single `all` row — matching the
 * web builder (SegmentForm.tsx:37-55), and matching the engine, which already
 * treats an empty rule list as "everyone".
 */
export function parseSegmentRules(raw: string | undefined | null): RulesDraft {
  const text = (raw ?? "").trim();
  if (text.length === 0) return { mode: "rows", rows: [newRuleRow("r0")] };

  let parsed: unknown;
  try {
    parsed = JSON.parse(text);
  } catch {
    return { mode: "opaque", raw: text };
  }
  if (!Array.isArray(parsed)) return { mode: "opaque", raw: text };
  if (parsed.length === 0) return { mode: "rows", rows: [newRuleRow("r0")] };

  const rows = parsed.map((entry, index): RuleRow => {
    const id = `r${index}`;
    if (isPlainObject(entry) && isRuleType(entry.type)) {
      return {
        kind: "known",
        id,
        type: entry.type,
        field: stringField(entry.field),
        value: stringField(entry.value),
      };
    }
    return { kind: "unsupported", id, raw: entry, summary: JSON.stringify(entry) ?? "null" };
  });
  return { mode: "rows", rows };
}

/** One row, as the Go struct sees it. */
export function ruleToWire(row: KnownRuleRow): WireSegmentRule {
  return {
    type: row.type,
    field: row.field,
    // Never carry a value onto a type that has none: a merchant who typed 90
    // and then switched the row to "Everyone" must not ship a stray "90".
    value: ruleTakesValue(row.type) ? row.value.trim() : "",
  };
}

/**
 * The string sent as `rules`.
 *
 * Known rows serialise to all three keys as strings — the `SegmentRule`
 * contract. Unsupported rows re-emit their original JSON value unchanged.
 */
export function serializeSegmentRules(draft: RulesDraft): string {
  if (draft.mode === "opaque") return draft.raw;
  return JSON.stringify(
    draft.rows.map((row) => (row.kind === "known" ? ruleToWire(row) : row.raw)),
  );
}

/**
 * Why this draft cannot be saved yet, in words a merchant can act on — or
 * null.
 *
 * The failure this replaces was `Couldn't create segment / Please try again`
 * with no field named and no reason given, on a form whose own example was
 * invalid. Every message below names the rule NUMBER.
 */
export function segmentRulesError(draft: RulesDraft): string | null {
  if (draft.mode === "opaque") return null;
  if (draft.rows.length === 0) return "Add at least one rule.";

  for (let i = 0; i < draft.rows.length; i += 1) {
    const row = draft.rows[i]!;
    if (row.kind !== "known") continue;
    const position = i + 1;
    const value = row.value.trim();
    if (row.type === "loyalty_tier" && value.length === 0) {
      return `Rule ${position}: choose a loyalty tier.`;
    }
    if (row.type === "inactive_days") {
      if (value.length === 0) return `Rule ${position}: enter a number of days.`;
      const days = Number(value);
      if (!Number.isInteger(days) || days < 1) {
        return `Rule ${position}: days must be a whole number of 1 or more.`;
      }
    }
  }
  return null;
}
