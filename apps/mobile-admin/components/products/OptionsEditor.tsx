import { useState } from "react";
import { View, TextInput, Pressable, StyleSheet } from "react-native";
import { X } from "lucide-react-native";
import { Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { ProductOption } from "@repo/mobile-shared/api/schemas/products";
import type { UpdateProductOptionBody } from "@repo/mobile-shared/api/products";

/**
 * Converts the RESPONSE option shape into the REQUEST one.
 *
 * 🔴 The response sends `values: [{id, value, position}]`; the request wants
 * `values: string[]`. Same field name, two shapes — the single most expensive
 * recurring bug on this project. This function is the ONLY place that bridges
 * them, and __tests__/options-editor.test.tsx pins both sides.
 *
 * Values are ordered by `position`: the wire does not guarantee array order
 * (variants demonstrably come back 2,3,4,0,1).
 */
export function toOptionRequestBodies(options: ProductOption[]): UpdateProductOptionBody[] {
  return [...options]
    .sort((a, b) => a.position - b.position)
    .map((o) => ({
      name: o.name,
      values: [...o.values].sort((a, b) => a.position - b.position).map((v) => v.value),
    }));
}

interface OptionsEditorProps {
  options: ProductOption[];
  /**
   * Receives the COMPLETE desired option set, never a delta — sending `options`
   * routes the PATCH through UpdateAggregate, whose applyOptionsDiff reconciles
   * the whole variant matrix against what it is given.
   */
  onChange: (options: UpdateProductOptionBody[]) => void;
}

export function OptionsEditor({ options, onChange }: OptionsEditorProps) {
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const current = toOptionRequestBodies(options);

  const addValue = (optionName: string) => {
    const raw = drafts[optionName] ?? "";
    const value = raw.trim();
    if (value === "") return;
    const target = current.find((o) => o.name === optionName);
    if (!target || target.values.includes(value)) return;
    setDrafts((d) => ({ ...d, [optionName]: "" }));
    onChange(
      current.map((o) => (o.name === optionName ? { ...o, values: [...o.values, value] } : o)),
    );
  };

  const removeValue = (optionName: string, value: string) => {
    onChange(
      current.map((o) =>
        o.name === optionName ? { ...o, values: o.values.filter((v) => v !== value) } : o,
      ),
    );
  };

  return (
    <View style={styles.root}>
      {current.map((option) => (
        <View key={option.name} style={styles.option}>
          <Text preset="bodyEmphasis" color="text">
            {option.name}
          </Text>
          <View style={styles.chips}>
            {option.values.map((value) => (
              <View key={value} style={styles.chip}>
                <Text preset="caption" color="text">
                  {value}
                </Text>
                <Pressable
                  onPress={() => removeValue(option.name, value)}
                  accessibilityRole="button"
                  accessibilityLabel={`Remove ${value} from ${option.name}`}
                  hitSlop={8}
                >
                  <X size={12} color={theme.colors.textTertiary} strokeWidth={2.5} />
                </Pressable>
              </View>
            ))}
          </View>
          <TextInput
            style={styles.input}
            value={drafts[option.name] ?? ""}
            onChangeText={(t) => setDrafts((d) => ({ ...d, [option.name]: t }))}
            onSubmitEditing={() => addValue(option.name)}
            placeholder={`Add a ${option.name.toLowerCase()}…`}
            placeholderTextColor={theme.colors.textTertiary}
            accessibilityLabel={`Add a value to ${option.name}`}
            returnKeyType="done"
          />
        </View>
      ))}
    </View>
  );
}

const styles = StyleSheet.create({
  root: { gap: theme.spacing.md },
  option: { gap: theme.spacing.sm },
  chips: { flexDirection: "row", flexWrap: "wrap", gap: theme.spacing.xs },
  chip: {
    flexDirection: "row",
    alignItems: "center",
    gap: theme.spacing.xs,
    paddingHorizontal: theme.spacing.sm,
    height: 32,
    borderRadius: theme.radii.sm,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    backgroundColor: theme.colors.elevated,
  },
  input: {
    height: 44,
    borderWidth: theme.hairline,
    borderColor: theme.colors.border,
    borderRadius: theme.radii.md,
    paddingHorizontal: theme.spacing.sm,
    color: theme.colors.text,
    backgroundColor: theme.colors.elevated,
  },
});
