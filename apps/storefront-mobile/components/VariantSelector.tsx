import { View, Text, StyleSheet, Pressable } from "react-native";
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

const styles = StyleSheet.create({
  container: {
    gap: 16,
  },
  optionGroup: {
    gap: 8,
  },
  optionLabel: {
    fontSize: 13,
    fontWeight: "600",
    color: "#0E0E0C",
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
    borderColor: "#E5E4DF",
    backgroundColor: "#FFFFFF",
  },
  chipSelected: {
    borderColor: "#0E0E0C",
    backgroundColor: "#0E0E0C",
  },
  chipUnavailable: {
    borderColor: "#E5E4DF",
    backgroundColor: "#F7F6F2",
    opacity: 0.5,
  },
  chipText: {
    fontSize: 14,
    color: "#0E0E0C",
    fontWeight: "500",
  },
  chipTextSelected: {
    color: "#FFFFFF",
  },
  chipTextUnavailable: {
    color: "#999999",
    textDecorationLine: "line-through",
  },
});
