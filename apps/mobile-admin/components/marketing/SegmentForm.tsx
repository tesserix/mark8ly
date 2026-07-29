import { useCallback, useRef, useState } from "react";
import { View, Platform, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { ChevronDown } from "lucide-react-native";
import { ActionSheet, FieldInput, FieldLabel, Hairline, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import {
  RULE_TYPES,
  RULE_TYPE_HINT,
  RULE_TYPE_LABEL,
  newRuleRow,
  parseSegmentRules,
  ruleTakesValue,
  segmentRulesError,
  serializeSegmentRules,
  type RuleRow,
  type RulesDraft,
  type SegmentRuleType,
} from "./segment-rules";

export interface SegmentFormValues {
  name: string;
  description?: string;
  /** The serialised `[{type, field, value}]` array — see `segment-rules.ts`. */
  rules: string;
}

interface SegmentFormProps {
  initialName?: string;
  initialDescription?: string;
  initialRules?: string;
  /**
   * Loyalty tier NAMES, for the `loyalty_tier` rule's picker. Additive and
   * optional: with none configured the row falls back to a free-text field
   * rather than blocking the rule outright, because `customer_loyalties.tier`
   * is matched as a plain string and a store can hold tiers the program
   * config no longer lists.
   */
  tiers?: string[];
  submitLabel: string;
  isSubmitting?: boolean;
  onSubmit: (values: SegmentFormValues) => void;
}

/**
 * Shared create/edit form for a customer segment.
 *
 * The rules field used to be a raw JSON textarea whose own placeholder was
 * invalid — it advertised an `operator` key the Go model has never had and a
 * NUMERIC `value` the model rejects — and on iOS smart punctuation curled the
 * quotes of anything typed into it, so the JSON was unparseable before it
 * ever left the phone. React Native exposes no prop that turns smart
 * punctuation off (`autoCorrect={false}` does not), so the textarea could not
 * be repaired; it had to go. This is the web builder's model
 * (apps/admin/components/marketing/segments/SegmentForm.tsx), ported.
 */
export function SegmentForm({
  initialName = "",
  initialDescription = "",
  initialRules = "",
  tiers = [],
  submitLabel,
  isSubmitting = false,
  onSubmit,
}: SegmentFormProps) {
  const [name, setName] = useState(initialName);
  const [description, setDescription] = useState(initialDescription);
  const [draft, setDraft] = useState<RulesDraft>(() => parseSegmentRules(initialRules));

  // Row ids are stable React keys, never indexes: removing rule 1 must not
  // hand rule 2's key to rule 3 and carry its input state across.
  const nextId = useRef(draft.mode === "rows" ? draft.rows.length : 0);
  const mintId = useCallback(() => {
    const id = `r${nextId.current}`;
    nextId.current += 1;
    return id;
  }, []);

  const [typePickerFor, setTypePickerFor] = useState<string | null>(null);
  const [tierPickerFor, setTierPickerFor] = useState<string | null>(null);

  const updateRow = useCallback((id: string, patch: (row: RuleRow) => RuleRow) => {
    setDraft((prev) =>
      prev.mode === "rows"
        ? { mode: "rows", rows: prev.rows.map((row) => (row.id === id ? patch(row) : row)) }
        : prev,
    );
  }, []);

  const setRowType = useCallback(
    (id: string, type: SegmentRuleType) =>
      updateRow(id, (row) => (row.kind === "known" ? { ...row, type } : row)),
    [updateRow],
  );

  const setRowValue = useCallback(
    (id: string, value: string) =>
      updateRow(id, (row) => (row.kind === "known" ? { ...row, value } : row)),
    [updateRow],
  );

  const addRow = useCallback(() => {
    const id = mintId();
    setDraft((prev) =>
      prev.mode === "rows" ? { mode: "rows", rows: [...prev.rows, newRuleRow(id)] } : prev,
    );
  }, [mintId]);

  const removeRow = useCallback((id: string) => {
    setDraft((prev) =>
      prev.mode === "rows"
        ? { mode: "rows", rows: prev.rows.filter((row) => row.id !== id) }
        : prev,
    );
  }, []);

  /** The only route out of `opaque` — and it is the merchant's own choice,
   *  never something save does behind their back. */
  const replaceOpaqueRules = useCallback(() => {
    nextId.current = 1;
    setDraft({ mode: "rows", rows: [newRuleRow("r0")] });
  }, []);

  const rulesError = segmentRulesError(draft);
  const canSubmit = name.trim().length > 0 && rulesError === null && !isSubmitting;

  const submit = useCallback(() => {
    if (!canSubmit) return;
    onSubmit({
      name: name.trim(),
      ...(description.trim() ? { description: description.trim() } : {}),
      rules: serializeSegmentRules(draft),
    });
  }, [canSubmit, name, description, draft, onSubmit]);

  const rows = draft.mode === "rows" ? draft.rows : [];
  const canRemove = rows.length > 1;
  const hasTiers = tiers.length > 0;

  return (
    <View style={styles.form}>
      <FieldInput label="Name" value={name} onChangeText={setName} placeholder="High spenders" />
      <FieldInput
        label="Description (optional)"
        value={description}
        onChangeText={setDescription}
        placeholder="What this segment is for"
        multiline
      />

      <Hairline />

      <View style={styles.rulesHead}>
        <Text preset="bodyEmphasis" color="text">
          Who's in it
        </Text>
        {draft.mode === "rows" ? (
          <AddRuleButton onPress={addRow} />
        ) : null}
      </View>
      <Text preset="caption" color="textTertiary">
        A customer has to match every rule you add.
      </Text>

      {draft.mode === "opaque" ? (
        <View style={styles.opaque} testID="segment-rules-opaque">
          <Text preset="caption" color="textSecondary">
            These rules were saved in a format this screen can't show. They're kept exactly as
            they are — saving a new name or description won't change them.
          </Text>
          <Text preset="caption" color="textTertiary" style={styles.raw}>
            {draft.raw}
          </Text>
          <Pressable
            onPress={replaceOpaqueRules}
            accessibilityRole="button"
            accessibilityLabel="Replace these rules"
            testID="segment-rules-replace"
            style={styles.replaceBtn}
          >
            <Text preset="caption" color="danger">
              Replace them
            </Text>
          </Pressable>
        </View>
      ) : (
        rows.map((row, index) => (
          <View key={row.id} style={styles.rule} testID={`segment-rule-${index}`}>
            <View style={styles.ruleHead}>
              <Text preset="caption" color="textTertiary">
                {`Rule ${index + 1}`}
              </Text>
              {canRemove ? (
                <RemoveRuleButton index={index} onPress={() => removeRow(row.id)} />
              ) : null}
            </View>

            {row.kind === "known" ? (
              <>
                <PickerField
                  label="Rule"
                  display={RULE_TYPE_LABEL[row.type]}
                  onPress={() => setTypePickerFor(row.id)}
                  testID={`segment-rule-${index}-type`}
                  accessibilityLabel={`Rule ${index + 1} type: ${RULE_TYPE_LABEL[row.type]}`}
                />
                <Text preset="caption" color="textTertiary">
                  {RULE_TYPE_HINT[row.type]}
                </Text>

                {row.type === "loyalty_tier" ? (
                  hasTiers ? (
                    <PickerField
                      label="Tier"
                      display={row.value}
                      placeholder="Choose a tier"
                      onPress={() => setTierPickerFor(row.id)}
                      testID={`segment-rule-${index}-tier`}
                      accessibilityLabel={
                        row.value
                          ? `Rule ${index + 1} tier: ${row.value}`
                          : `Rule ${index + 1}: choose a tier`
                      }
                    />
                  ) : (
                    <FieldInput
                      label="Tier"
                      value={row.value}
                      onChangeText={(text) => setRowValue(row.id, text)}
                      placeholder="gold"
                      autoCapitalize="none"
                      testID={`segment-rule-${index}-value`}
                      accessibilityLabel={`Rule ${index + 1} tier name`}
                    />
                  )
                ) : null}

                {row.type === "inactive_days" ? (
                  <FieldInput
                    label="Days"
                    value={row.value}
                    onChangeText={(text) => setRowValue(row.id, text)}
                    placeholder="90"
                    // number-pad, not decimal-pad: the engine does
                    // strconv.Atoi on this and a "90.5" is a 500 at send time.
                    keyboardType="number-pad"
                    testID={`segment-rule-${index}-value`}
                    accessibilityLabel={`Rule ${index + 1} days`}
                  />
                ) : null}
              </>
            ) : (
              <View testID={`segment-rule-${index}-unsupported`} style={styles.unsupported}>
                <Text preset="caption" color="textSecondary">
                  This rule was set up somewhere else. It still applies — it's kept exactly as it
                  is unless you remove it.
                </Text>
                <Text preset="caption" color="textTertiary" style={styles.raw}>
                  {row.summary}
                </Text>
              </View>
            )}
          </View>
        ))
      )}

      {rulesError ? (
        <Text preset="caption" color="danger" testID="segment-rules-error">
          {rulesError}
        </Text>
      ) : null}

      <Pressable
        style={[styles.btn, !canSubmit && styles.disabled]}
        onPress={submit}
        disabled={!canSubmit}
        accessibilityRole="button"
        accessibilityLabel={submitLabel}
        testID="segment-submit"
      >
        {isSubmitting ? (
          <ActivityIndicator size="small" color={theme.colors.inverse} />
        ) : (
          <Text preset="bodyEmphasis" color="inverse">
            {submitLabel}
          </Text>
        )}
      </Pressable>

      <ActionSheet
        title="Rule"
        items={RULE_TYPES.map((type) => ({
          key: `type-${type}`,
          label: RULE_TYPE_LABEL[type],
          onPress: () => {
            if (typePickerFor) setRowType(typePickerFor, type);
          },
        }))}
        visible={typePickerFor !== null}
        onDismiss={() => setTypePickerFor(null)}
      />

      <ActionSheet
        title="Loyalty tier"
        items={tiers.map((tier) => ({
          key: `tier-${tier}`,
          label: tier,
          onPress: () => {
            if (tierPickerFor) setRowValue(tierPickerFor, tier);
          },
        }))}
        visible={tierPickerFor !== null}
        onDismiss={() => setTierPickerFor(null)}
      />
    </View>
  );
}

/**
 * A tap-to-choose field.
 *
 * Deliberately a COLUMN block, not a picker sitting beside its value on one
 * line: a fixed box holding scalable text is the shape that has produced
 * eight silent-clipping bugs in this app. The label text carries no
 * `numberOfLines`, so at `accessibility-large` "Hasn't ordered in a while"
 * wraps and the box grows with it instead of truncating.
 *
 * Press state is `useState`, and `style` is a plain ARRAY — never the
 * `({pressed}) => …` callback form, which NativeWind's JSX interop silently
 * drops at runtime while every RNTL test stays green.
 */
function PickerField({
  label,
  display,
  placeholder,
  onPress,
  testID,
  accessibilityLabel,
}: {
  label: string;
  display: string;
  placeholder?: string;
  onPress: () => void;
  testID: string;
  accessibilityLabel: string;
}) {
  const [pressed, setPressed] = useState(false);
  const shown = display.trim().length > 0 ? display : (placeholder ?? "");
  return (
    <View style={styles.fieldWrap}>
      <FieldLabel label={label} />
      <Pressable
        onPress={onPress}
        onPressIn={() => setPressed(true)}
        onPressOut={() => setPressed(false)}
        accessibilityRole="button"
        accessibilityLabel={accessibilityLabel}
        testID={testID}
        android_ripple={theme.press.rippleInk}
        style={[
          styles.picker,
          pressed && Platform.OS === "ios" ? styles.pickerPressed : null,
        ]}
      >
        <Text
          preset="body"
          color={display.trim().length > 0 ? "text" : "textTertiary"}
          style={styles.pickerText}
        >
          {shown}
        </Text>
        <ChevronDown size={16} color={theme.colors.textTertiary} strokeWidth={1.75} />
      </Pressable>
    </View>
  );
}

function AddRuleButton({ onPress }: { onPress: () => void }) {
  const [pressed, setPressed] = useState(false);
  return (
    <Pressable
      onPress={onPress}
      onPressIn={() => setPressed(true)}
      onPressOut={() => setPressed(false)}
      accessibilityRole="button"
      accessibilityLabel="Add rule"
      testID="segment-add-rule"
      android_ripple={theme.press.rippleAccent}
      style={[
        styles.addBtn,
        pressed && Platform.OS === "ios" ? { opacity: theme.press.opacityStandard } : null,
      ]}
    >
      <Text preset="caption" color="accent">
        Add rule
      </Text>
    </Pressable>
  );
}

function RemoveRuleButton({ index, onPress }: { index: number; onPress: () => void }) {
  const [pressed, setPressed] = useState(false);
  return (
    <Pressable
      onPress={onPress}
      onPressIn={() => setPressed(true)}
      onPressOut={() => setPressed(false)}
      accessibilityRole="button"
      accessibilityLabel={`Remove rule ${index + 1}`}
      testID={`segment-rule-${index}-remove`}
      android_ripple={theme.press.rippleInk}
      style={[
        styles.removeBtn,
        pressed && Platform.OS === "ios" ? { opacity: theme.press.opacityStandard } : null,
      ]}
    >
      <Text preset="caption" color="textSecondary">
        Remove
      </Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  form: { gap: theme.spacing.md },
  rulesHead: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    // Wraps rather than crushes: at accessibility text sizes the heading and
    // the button no longer fit one line, and a `minWidth` box that cannot
    // wrap is where the clipping starts.
    flexWrap: "wrap",
    gap: theme.spacing.sm,
  },
  rule: {
    gap: theme.spacing.sm,
    padding: theme.spacing.md,
    borderRadius: theme.radii.md,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    backgroundColor: theme.colors.surfaceAlt,
  },
  ruleHead: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    flexWrap: "wrap",
    gap: theme.spacing.sm,
  },
  fieldWrap: { gap: theme.spacing.xs },
  picker: {
    flexDirection: "row",
    alignItems: "center",
    gap: theme.spacing.sm,
    // A real 44pt+ box, as a MINIMUM rather than a fixed height — the label
    // inside is allowed to wrap and push it taller.
    minHeight: theme.touchTarget + 4,
    paddingHorizontal: theme.spacing.sm,
    paddingVertical: theme.spacing.sm,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.elevated,
  },
  pickerPressed: { backgroundColor: theme.colors.sink },
  pickerText: { flex: 1 },
  addBtn: {
    minHeight: theme.touchTarget,
    minWidth: theme.touchTarget,
    justifyContent: "center",
    alignItems: "flex-end",
  },
  removeBtn: {
    minHeight: theme.touchTarget,
    minWidth: theme.touchTarget,
    justifyContent: "center",
    alignItems: "flex-end",
  },
  unsupported: { gap: theme.spacing.xs },
  opaque: {
    gap: theme.spacing.sm,
    padding: theme.spacing.md,
    borderRadius: theme.radii.md,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    backgroundColor: theme.colors.surfaceAlt,
  },
  replaceBtn: {
    minHeight: theme.touchTarget,
    justifyContent: "center",
  },
  raw: { fontFamily: theme.fonts.mono },
  btn: {
    height: 48,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.text,
    alignItems: "center",
    justifyContent: "center",
    marginTop: theme.spacing.sm,
  },
  disabled: { opacity: 0.4 },
});
