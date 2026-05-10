import { View, TextInput, StyleSheet, TouchableOpacity, type ViewStyle } from "react-native";
import { Search, X } from "lucide-react-native";
import { theme } from "@/lib/theme";

interface SearchFieldProps {
  value: string;
  onChangeText: (text: string) => void;
  placeholder?: string;
  style?: ViewStyle;
  accessibilityLabel?: string;
}

export function SearchField({
  value,
  onChangeText,
  placeholder = "Search...",
  style,
  accessibilityLabel,
}: SearchFieldProps) {
  return (
    <View style={[styles.wrapper, style]}>
      <Search size={16} color={theme.colors.textTertiary} strokeWidth={1.75} />
      <TextInput
        style={styles.input}
        placeholder={placeholder}
        placeholderTextColor={theme.colors.textTertiary}
        value={value}
        onChangeText={onChangeText}
        autoCapitalize="none"
        autoCorrect={false}
        returnKeyType="search"
        accessibilityLabel={accessibilityLabel ?? placeholder}
        clearButtonMode="never"
      />
      {value.length > 0 ? (
        <TouchableOpacity
          onPress={() => onChangeText("")}
          hitSlop={12}
          accessibilityRole="button"
          accessibilityLabel="Clear search"
        >
          <X size={16} color={theme.colors.textTertiary} strokeWidth={1.75} />
        </TouchableOpacity>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  wrapper: {
    flexDirection: "row",
    alignItems: "center",
    gap: theme.spacing.sm,
    backgroundColor: theme.colors.elevated,
    borderRadius: theme.radii.md,
    borderWidth: theme.hairline,
    borderColor: theme.colors.hairline,
    paddingHorizontal: theme.spacing.md,
    height: 40,
  },
  input: {
    flex: 1,
    height: "100%",
    fontFamily: theme.fonts.sans,
    fontSize: 14,
    color: theme.colors.text,
    paddingVertical: 0,
  },
});
