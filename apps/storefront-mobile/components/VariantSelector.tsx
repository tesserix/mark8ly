import { useMemo } from "react";
import { View, Text, StyleSheet, Pressable } from "react-native";
import { useTheme } from "@/lib/theme/theme-provider";
import type {
  StorefrontProductOption,
  StorefrontVariant,
} from "@repo/mobile-shared/api/storefront-types";

interface VariantSelectorProps {
  options: StorefrontProductOption[];
  variants: StorefrontVariant[];
  selectedValues: Record<string, string>;
  onSelect: (optionName: string, value: string) => void;
}

function isValueAvailable(
  optionName: string,
  value: string,
  selectedValues: Record<string, string>,
  variants: StorefrontVariant[],
): boolean {
  const testSelection = { ...selectedValues, [optionName]: value };
  return variants.some((variant) => {
    const matches = Object.entries(testSelection).every(
      ([key, val]) => variant.option_values[key] === val,
    );
    return matches && variant.stock_status !== "out_of_stock";
  });
}

export function VariantSelector({
  options,
  variants,
  selectedValues,
  onSelect,
}: VariantSelectorProps) {
  const theme = useTheme();
  const styles = useMemo(() => createThemedStyles(theme), [theme]);
  if (options.length === 0) return null;

  return (
    <View style={styles.container}>
      {options.map((option) => (
        <View key={option.name} style={styles.optionGroup}>
          <Text style={styles.optionLabel}>{option.name}</Text>
          <View style={styles.chips}>
            {option.values.map((value) => {
              const isSelected = selectedValues[option.name] === value;
              const available = isValueAvailable(
                option.name,
                value,
                selectedValues,
                variants,
              );

              return (
                <Pressable
                  key={value}
                  style={[
                    styles.chip,
                    isSelected && styles.chipSelected,
                    !available && styles.chipUnavailable,
                  ]}
                  onPress={() => available && onSelect(option.name, value)}
                  disabled={!available}
                >
                  <Text
                    style={[
                      styles.chipText,
                      isSelected && styles.chipTextSelected,
                      !available && styles.chipTextUnavailable,
                    ]}
                  >
                    {value}
                  </Text>
                </Pressable>
              );
            })}
          </View>
        </View>
      ))}
    </View>
  );
}

function createThemedStyles(theme: { primary: string; accent: string; background: string; elevated: string; text: string; textSecondary: string; border: string; fontFamily: string }) {
  return StyleSheet.create({
  container: {
    gap: 16,
  },
  optionGroup: {
    gap: 8,
  },
  optionLabel: {
    fontSize: 13,
    fontWeight: "600",
    color: theme.text,
    textTransform: "uppercase",
    letterSpacing: 0.5,
  },
  chips: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
  },
  chip: {
    paddingHorizontal: 16,
    paddingVertical: 10,
    borderRadius: 6,
    borderWidth: 1,
    borderColor: theme.border,
    backgroundColor: theme.elevated,
  },
  chipSelected: {
    borderColor: theme.primary,
    backgroundColor: theme.primary,
  },
  chipUnavailable: {
    borderColor: theme.border,
    backgroundColor: theme.background,
    opacity: 0.5,
  },
  chipText: {
    fontSize: 14,
    color: theme.text,
    fontWeight: "500",
  },
  chipTextSelected: {
    color: theme.elevated,
  },
  chipTextUnavailable: {
    color: theme.textSecondary,
    textDecorationLine: "line-through",
  },
});
}
