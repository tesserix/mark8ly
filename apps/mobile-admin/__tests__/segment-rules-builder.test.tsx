// Segments — the structured rules builder that replaced the JSON textarea
// (inc3 Task 17).
//
// The defect being fixed was a client that invented its own schema: the form's
// own placeholder was `[{"field":"total_spent","operator":"gt","value":100}]`,
// which has an `operator` key `SegmentRule` has never had, a `total_spent`
// type the engine does not know, and a NUMERIC `value` where the Go model
// requires a string — so a merchant who copied the app's own example got a
// 400, and one who "fixed" it by quoting the number persisted a segment with
// an empty `Type` that matches nobody.
//
// Every assertion below is therefore pinned to the WIRE SHAPE
// (services/marketplace-api/internal/campaign/models.go:116-133) — three
// string fields, four type values — not to whatever the form happens to emit.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));

jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});

jest.mock("@repo/mobile-shared/haptics/feedback", () => ({
  adminHaptics: {
    actionSucceeded: jest.fn(() => Promise.resolve()),
    actionFailed: jest.fn(() => Promise.resolve()),
    swipeThreshold: jest.fn(() => Promise.resolve()),
    menuOpen: jest.fn(() => Promise.resolve()),
    selectionChanged: jest.fn(() => Promise.resolve()),
  },
}));

import { fireEvent, render } from "@testing-library/react-native";
import { SegmentForm } from "@/components/marketing/SegmentForm";
import {
  parseSegmentRules,
  segmentRulesError,
  serializeSegmentRules,
  type RulesDraft,
} from "@/components/marketing/segment-rules";

/** What the old, broken mobile placeholder actually produced. */
const BROKEN_PLACEHOLDER = '[{"field":"total_spent","operator":"gt","value":100}]';

/** What the WEB builder writes — `{type, value?}`, no `field` key. */
const WEB_RULES = '[{"type":"loyalty_tier","value":"gold"}]';

const TIERS = ["bronze", "silver", "gold"];

function draftOf(raw: string): RulesDraft {
  return parseSegmentRules(raw);
}

/** The rules string as an array of wire objects. */
function wire(rules: string): unknown[] {
  return JSON.parse(rules) as unknown[];
}

// --- the pure model -----------------------------------------------------

describe("segment rules — parse", () => {
  it("opens on a single all-customers rule when there are no rules yet", () => {
    for (const empty of ["", "   ", undefined]) {
      const draft = parseSegmentRules(empty);
      expect(draft).toEqual({
        mode: "rows",
        rows: [{ kind: "known", id: "r0", type: "all", field: "", value: "" }],
      });
    }
  });

  it("treats a stored empty array the same way", () => {
    expect(parseSegmentRules("[]")).toEqual({
      mode: "rows",
      rows: [{ kind: "known", id: "r0", type: "all", field: "", value: "" }],
    });
  });

  it("reads each of the four supported types back as an editable row", () => {
    const draft = parseSegmentRules(
      '[{"type":"all"},{"type":"has_ordered"},{"type":"loyalty_tier","value":"gold"},{"type":"inactive_days","value":"90"}]',
    );
    expect(draft.mode).toBe("rows");
    if (draft.mode !== "rows") throw new Error("unreachable");
    expect(draft.rows.map((r) => (r.kind === "known" ? r.type : "?"))).toEqual([
      "all",
      "has_ordered",
      "loyalty_tier",
      "inactive_days",
    ]);
  });

  // The stored rule cannot round-trip through the Go model at all
  // (`json: cannot unmarshal number into Go struct field .value of type
  // string`), so coercing it to "90" REPAIRS it — and the merchant sees the
  // repaired value in the field before they save it.
  it("coerces a numeric value to the string the Go model requires", () => {
    const draft = parseSegmentRules('[{"type":"inactive_days","value":90}]');
    if (draft.mode !== "rows") throw new Error("unreachable");
    const row = draft.rows[0]!;
    expect(row.kind).toBe("known");
    if (row.kind !== "known") throw new Error("unreachable");
    expect(row.value).toBe("90");
  });

  it("keeps a stored `field` qualifier rather than rewriting it", () => {
    const draft = parseSegmentRules('[{"type":"loyalty_tier","field":"tier","value":"gold"}]');
    if (draft.mode !== "rows") throw new Error("unreachable");
    const row = draft.rows[0]!;
    if (row.kind !== "known") throw new Error("unreachable");
    expect(row.field).toBe("tier");
  });

  // The decision this whole file exists to pin: a rule the builder cannot
  // represent is NEITHER dropped NOR coerced to `all`. Both silently change
  // who the segment targets.
  it("preserves a rule it cannot represent instead of dropping or coercing it", () => {
    const draft = parseSegmentRules(BROKEN_PLACEHOLDER);
    if (draft.mode !== "rows") throw new Error("unreachable");
    expect(draft.rows).toHaveLength(1);
    expect(draft.rows[0]!.kind).toBe("unsupported");
    // Not silently turned into "everyone", which is what the web builder does.
    expect(draft.rows.some((r) => r.kind === "known")).toBe(false);
  });

  it("preserves a mix of known and unknown rules in order", () => {
    const draft = parseSegmentRules(
      '[{"type":"has_ordered"},{"type":"total_spent","operator":"gt","value":"100"}]',
    );
    if (draft.mode !== "rows") throw new Error("unreachable");
    expect(draft.rows.map((r) => r.kind)).toEqual(["known", "unsupported"]);
  });

  it("falls back to opaque for rules that are not even a JSON array", () => {
    expect(parseSegmentRules("not json at all")).toEqual({
      mode: "opaque",
      raw: "not json at all",
    });
    expect(parseSegmentRules('{"type":"all"}')).toEqual({
      mode: "opaque",
      raw: '{"type":"all"}',
    });
  });
});

describe("segment rules — serialise to the SegmentRule wire shape", () => {
  it("emits exactly {type, field, value}, all strings, for every type", () => {
    const draft = draftOf(
      '[{"type":"all"},{"type":"has_ordered"},{"type":"loyalty_tier","value":"gold"},{"type":"inactive_days","value":"90"}]',
    );
    const rules = wire(serializeSegmentRules(draft));
    expect(rules).toEqual([
      { type: "all", field: "", value: "" },
      { type: "has_ordered", field: "", value: "" },
      { type: "loyalty_tier", field: "", value: "gold" },
      { type: "inactive_days", field: "", value: "90" },
    ]);
    for (const rule of rules as Record<string, unknown>[]) {
      expect(Object.keys(rule).sort()).toEqual(["field", "type", "value"]);
      expect(typeof rule.type).toBe("string");
      expect(typeof rule.field).toBe("string");
      expect(typeof rule.value).toBe("string");
      // The exact defect: `operator` is not part of the contract.
      expect(rule).not.toHaveProperty("operator");
    }
  });

  // `strconv.Atoi(rule.Value)` — a number here is a 400 at create time,
  // because the Go field is a `string`.
  it("emits inactive_days as the STRING \"90\", never the number 90", () => {
    const rules = wire(serializeSegmentRules(draftOf('[{"type":"inactive_days","value":90}]')));
    const value = (rules[0] as { value: unknown }).value;
    expect(value).toBe("90");
    expect(typeof value).toBe("string");
    expect(value).not.toBe(90);
  });

  it("never carries a stray value onto a type that has none", () => {
    // A merchant who typed 90, then switched the row to "Everyone".
    const draft = draftOf('[{"type":"inactive_days","value":"90"}]');
    if (draft.mode !== "rows") throw new Error("unreachable");
    const row = draft.rows[0]!;
    if (row.kind !== "known") throw new Error("unreachable");
    const switched: RulesDraft = { mode: "rows", rows: [{ ...row, type: "all" }] };
    expect(wire(serializeSegmentRules(switched))).toEqual([
      { type: "all", field: "", value: "" },
    ]);
  });

  it("trims a value before sending it", () => {
    const draft = draftOf('[{"type":"loyalty_tier","value":"  gold  "}]');
    expect((wire(serializeSegmentRules(draft))[0] as { value: string }).value).toBe("gold");
  });

  it("re-emits an unrepresentable rule byte-for-byte", () => {
    expect(serializeSegmentRules(draftOf(BROKEN_PLACEHOLDER))).toBe(BROKEN_PLACEHOLDER);
  });

  it("re-emits opaque rules verbatim", () => {
    expect(serializeSegmentRules(draftOf("not json at all"))).toBe("not json at all");
  });

  it("round-trips its own output byte-for-byte", () => {
    const once = serializeSegmentRules(
      draftOf(
        '[{"type":"all"},{"type":"loyalty_tier","value":"gold"},{"type":"inactive_days","value":"90"}]',
      ),
    );
    expect(serializeSegmentRules(draftOf(once))).toBe(once);
  });

  it("keeps a web-written rule's MEANING on a re-save", () => {
    const out = wire(serializeSegmentRules(draftOf(WEB_RULES)));
    // The web omits `field`; this builder always writes all three keys. The
    // engine reads Type and Value only, so the audience is identical.
    expect(out).toEqual([{ type: "loyalty_tier", field: "", value: "gold" }]);
  });
});

describe("segment rules — what stops a save, and what it says", () => {
  it("says nothing about a complete draft", () => {
    expect(segmentRulesError(draftOf('[{"type":"all"}]'))).toBeNull();
    expect(segmentRulesError(draftOf('[{"type":"inactive_days","value":"90"}]'))).toBeNull();
  });

  it("names the rule that is missing a tier", () => {
    const err = segmentRulesError(draftOf('[{"type":"all"},{"type":"loyalty_tier"}]'));
    expect(err).toMatch(/Rule 2/);
    expect(err).toMatch(/tier/i);
  });

  it("names the rule whose day count is not a whole number of 1 or more", () => {
    for (const bad of ["", "0", "-5", "90.5", "ninety"]) {
      const err = segmentRulesError(
        draftOf(JSON.stringify([{ type: "inactive_days", value: bad }])),
      );
      expect(err).toMatch(/Rule 1/);
    }
  });

  it("requires at least one rule", () => {
    expect(segmentRulesError({ mode: "rows", rows: [] })).toMatch(/at least one/i);
  });

  it("never blocks a save on rules it cannot read", () => {
    expect(segmentRulesError(draftOf(BROKEN_PLACEHOLDER))).toBeNull();
    expect(segmentRulesError(draftOf("not json at all"))).toBeNull();
  });
});

// --- the form -----------------------------------------------------------

function renderForm(props: Partial<React.ComponentProps<typeof SegmentForm>> = {}) {
  const onSubmit = jest.fn();
  const root = render(
    <SegmentForm submitLabel="Create segment" tiers={TIERS} onSubmit={onSubmit} {...props} />,
  );
  return { root, onSubmit };
}

/** Open rule `index`'s type picker and choose `type`. */
function chooseType(root: ReturnType<typeof render>, index: number, type: string) {
  fireEvent.press(root.getByTestId(`segment-rule-${index}-type`));
  fireEvent.press(root.getByTestId(`action-sheet-item-type-${type}`));
}

describe("SegmentForm — the builder", () => {
  it("asks for no JSON anywhere, and no longer sends the merchant to the web", () => {
    const { root } = renderForm();
    expect(root.queryByText(/JSON/i)).toBeNull();
    expect(root.queryByText(/web dashboard/i)).toBeNull();
    expect(root.queryByPlaceholderText(/operator/i)).toBeNull();
    expect(root.queryByPlaceholderText(/total_spent/i)).toBeNull();
  });

  it("starts on one all-customers rule with no value field", () => {
    const { root } = renderForm();
    expect(root.getByTestId("segment-rule-0-type")).toBeTruthy();
    expect(root.queryByTestId("segment-rule-0-value")).toBeNull();
    expect(root.queryByTestId("segment-rule-0-tier")).toBeNull();
  });

  it("shows no value control for has_ordered either", () => {
    const { root } = renderForm();
    chooseType(root, 0, "has_ordered");
    expect(root.queryByTestId("segment-rule-0-value")).toBeNull();
    expect(root.queryByTestId("segment-rule-0-tier")).toBeNull();
  });

  it("offers a tier picker for loyalty_tier and a number field for inactive_days", () => {
    const { root } = renderForm();
    chooseType(root, 0, "loyalty_tier");
    expect(root.getByTestId("segment-rule-0-tier")).toBeTruthy();
    expect(root.queryByTestId("segment-rule-0-value")).toBeNull();

    chooseType(root, 0, "inactive_days");
    expect(root.queryByTestId("segment-rule-0-tier")).toBeNull();
    expect(root.getByTestId("segment-rule-0-value").props.keyboardType).toBe("number-pad");
  });

  it("falls back to a free-text tier field when the store has no tiers", () => {
    const { root } = renderForm({ tiers: [] });
    chooseType(root, 0, "loyalty_tier");
    expect(root.queryByTestId("segment-rule-0-tier")).toBeNull();
    expect(root.getByTestId("segment-rule-0-value")).toBeTruthy();
  });

  it("submits each of the four types in the SegmentRule shape", () => {
    const cases: [string, string | null, Record<string, string>][] = [
      ["all", null, { type: "all", field: "", value: "" }],
      ["has_ordered", null, { type: "has_ordered", field: "", value: "" }],
      ["loyalty_tier", "gold", { type: "loyalty_tier", field: "", value: "gold" }],
      ["inactive_days", "90", { type: "inactive_days", field: "", value: "90" }],
    ];
    for (const [type, value, expected] of cases) {
      const { root, onSubmit } = renderForm();
      fireEvent.changeText(root.getByPlaceholderText("High spenders"), "Test segment");
      chooseType(root, 0, type);
      if (type === "loyalty_tier") {
        fireEvent.press(root.getByTestId("segment-rule-0-tier"));
        fireEvent.press(root.getByTestId(`action-sheet-item-tier-${value}`));
      } else if (value !== null) {
        fireEvent.changeText(root.getByTestId("segment-rule-0-value"), value);
      }
      fireEvent.press(root.getByTestId("segment-submit"));

      expect(onSubmit).toHaveBeenCalledTimes(1);
      const sent = onSubmit.mock.calls[0]![0] as { name: string; rules: string };
      expect(sent.name).toBe("Test segment");
      expect(JSON.parse(sent.rules)).toEqual([expected]);
      root.unmount();
    }
  });

  it("adds and removes rules, and sends them all", () => {
    const { root, onSubmit } = renderForm();
    fireEvent.changeText(root.getByPlaceholderText("High spenders"), "Lapsed gold");

    // One rule: no remove control at all (the form requires at least one).
    expect(root.queryByTestId("segment-rule-0-remove")).toBeNull();

    fireEvent.press(root.getByTestId("segment-add-rule"));
    fireEvent.press(root.getByTestId("segment-add-rule"));
    chooseType(root, 0, "has_ordered");
    chooseType(root, 1, "inactive_days");
    fireEvent.changeText(root.getByTestId("segment-rule-1-value"), "90");
    chooseType(root, 2, "loyalty_tier");
    fireEvent.press(root.getByTestId("segment-rule-2-tier"));
    fireEvent.press(root.getByTestId("action-sheet-item-tier-gold"));

    // Drop the middle one — the remaining two must keep their own values,
    // which is what index-based React keys would get wrong.
    fireEvent.press(root.getByTestId("segment-rule-1-remove"));
    fireEvent.press(root.getByTestId("segment-submit"));

    const sent = onSubmit.mock.calls[0]![0] as { rules: string };
    expect(JSON.parse(sent.rules)).toEqual([
      { type: "has_ordered", field: "", value: "" },
      { type: "loyalty_tier", field: "", value: "gold" },
    ]);
  });

  it("refuses to submit an incomplete rule and says which one", () => {
    const { root, onSubmit } = renderForm();
    fireEvent.changeText(root.getByPlaceholderText("High spenders"), "Half-built");
    fireEvent.press(root.getByTestId("segment-add-rule"));
    chooseType(root, 1, "inactive_days");

    expect(root.getByTestId("segment-rules-error")).toHaveTextContent(/Rule 2/);
    expect(root.getByTestId("segment-submit").props.accessibilityState.disabled).toBe(true);
    fireEvent.press(root.getByTestId("segment-submit"));
    expect(onSubmit).not.toHaveBeenCalled();

    fireEvent.changeText(root.getByTestId("segment-rule-1-value"), "30");
    expect(root.queryByTestId("segment-rules-error")).toBeNull();
    fireEvent.press(root.getByTestId("segment-submit"));
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });

  it("still needs a name", () => {
    const { root, onSubmit } = renderForm();
    fireEvent.press(root.getByTestId("segment-submit"));
    expect(onSubmit).not.toHaveBeenCalled();
  });
});

describe("SegmentForm — editing an existing segment", () => {
  // The check that matters: a builder that rewrites rules on an unrelated
  // edit is worse than the textarea it replaced.
  it("leaves the rules alone when only the name changes", () => {
    const initial =
      '[{"type":"loyalty_tier","field":"","value":"gold"},{"type":"inactive_days","field":"","value":"90"}]';
    const { root, onSubmit } = renderForm({
      initialName: "Lapsed gold",
      initialRules: initial,
      submitLabel: "Save changes",
    });
    fireEvent.changeText(root.getByPlaceholderText("High spenders"), "Lapsed gold members");
    fireEvent.press(root.getByTestId("segment-submit"));

    const sent = onSubmit.mock.calls[0]![0] as { name: string; rules: string };
    expect(sent.name).toBe("Lapsed gold members");
    expect(sent.rules).toBe(initial);
  });

  it("keeps a rule it cannot represent when only the name changes", () => {
    const { root, onSubmit } = renderForm({
      initialName: "Legacy",
      initialRules: BROKEN_PLACEHOLDER,
      submitLabel: "Save changes",
    });
    // Shown, not hidden — the merchant can see there is a rule here.
    expect(root.getByTestId("segment-rule-0-unsupported")).toBeTruthy();

    fireEvent.changeText(root.getByPlaceholderText("High spenders"), "Legacy renamed");
    fireEvent.press(root.getByTestId("segment-submit"));

    const sent = onSubmit.mock.calls[0]![0] as { rules: string };
    expect(sent.rules).toBe(BROKEN_PLACEHOLDER);
  });

  it("lets a merchant replace an unrepresentable rule with a real one", () => {
    const { root, onSubmit } = renderForm({
      initialName: "Legacy",
      initialRules: `[{"type":"has_ordered"},${BROKEN_PLACEHOLDER.slice(1, -1)}]`,
      submitLabel: "Save changes",
    });
    fireEvent.press(root.getByTestId("segment-rule-1-remove"));
    fireEvent.press(root.getByTestId("segment-submit"));

    const sent = onSubmit.mock.calls[0]![0] as { rules: string };
    expect(JSON.parse(sent.rules)).toEqual([{ type: "has_ordered", field: "", value: "" }]);
  });

  it("shows unreadable rules read-only, and keeps them on a name-only save", () => {
    const { root, onSubmit } = renderForm({
      initialName: "Corrupt",
      initialRules: "not json at all",
      submitLabel: "Save changes",
    });
    expect(root.getByTestId("segment-rules-opaque")).toBeTruthy();
    expect(root.queryByTestId("segment-rule-0-type")).toBeNull();

    fireEvent.press(root.getByTestId("segment-submit"));
    expect((onSubmit.mock.calls[0]![0] as { rules: string }).rules).toBe("not json at all");

    // And the merchant can choose to start over — but only explicitly.
    fireEvent.press(root.getByTestId("segment-rules-replace"));
    expect(root.getByTestId("segment-rule-0-type")).toBeTruthy();
    fireEvent.press(root.getByTestId("segment-submit"));
    expect(JSON.parse((onSubmit.mock.calls[1]![0] as { rules: string }).rules)).toEqual([
      { type: "all", field: "", value: "" },
    ]);
  });
});
