import { View, TextInput, StyleSheet, type ViewStyle } from "react-native";
import { Search, X } from "lucide-react-native";
import { IconButton } from "./IconButton";
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
        <IconButton onPress={() => onChangeText("")} accessibilityLabel="Clear search">
          <X size={16} color={theme.colors.textTertiary} strokeWidth={1.75} />
        </IconButton>
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
    height: theme.touchTarget,
  },
  input: {
    flex: 1,
    height: "100%",
    fontFamily: theme.fonts.sans,
    // Was a literal 14 (the old body scale) — orphaned by the type rescale
    // to native metrics, so typed search text rendered a step smaller than
    // the rows beneath it. theme.text.body.fontSize stays anchored to the
    // current scale.
    fontSize: theme.text.body.fontSize,
    color: theme.colors.text,
    paddingVertical: 0,
  },
});
