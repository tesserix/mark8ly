import { useMemo } from "react";
import { View, Text, StyleSheet } from "react-native";
import { useTheme } from "@/lib/theme/theme-provider";
import { StarRating } from "./StarRating";

interface ReviewCardProps {
  rating: number;
  title: string;
  body: string;
  date: string;
}

function formatDate(iso: string): string {
  try {
    const d = new Date(iso);
    return d.toLocaleDateString("en-US", {
      month: "short",
      day: "numeric",
      year: "numeric",
    });
  } catch {
    return iso;
  }
}

export function ReviewCard({ rating, title, body, date }: ReviewCardProps) {
  const theme = useTheme();
  const styles = useMemo(() => createThemedStyles(theme), [theme]);
  return (
    <View style={styles.card}>
      <View style={styles.header}>
        <StarRating rating={rating} showCount={false} size={12} />
        <Text style={styles.date}>{formatDate(date)}</Text>
      </View>
      {title ? <Text style={styles.title}>{title}</Text> : null}
      {body ? (
        <Text style={styles.body} numberOfLines={4}>
          {body}
        </Text>
      ) : null}
    </View>
  );
}

function createThemedStyles(theme: { primary: string; accent: string; background: string; elevated: string; text: string; textSecondary: string; border: string; fontFamily: string }) {
  return StyleSheet.create({
  card: {
    paddingVertical: 14,
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: theme.border,
  },
  header: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    marginBottom: 6,
  },
  date: {
    fontSize: 12,
    color: theme.textSecondary,
  },
  title: {
    fontSize: 14,
    fontWeight: "600",
    color: theme.text,
    marginBottom: 4,
  },
  body: {
    fontSize: 13,
    color: theme.textSecondary,
    lineHeight: 18,
  },
});
}
