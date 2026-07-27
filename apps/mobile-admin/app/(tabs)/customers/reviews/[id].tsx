import { useCallback, useRef, useState } from "react";
import {
  Platform,
  View,
  ScrollView,
  Image,
  Pressable,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useLocalSearchParams } from "expo-router";
import { useReview } from "../../../../lib/hooks/use-reviews";
import {
  useApproveReview,
  useRejectReview,
  useToggleReviewFeatured,
  useReplyToReview,
} from "../../../../lib/admin-api/review-actions";
import { BackHeader, Eyebrow, Hairline, Screen, StatusBadge, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { ReviewStars } from "@/components/reviews/ReviewStars";
import { ReviewStatusBadge } from "@/components/reviews/ReviewStatusBadge";
import { ReviewReplySheet, type ReviewReplySheetHandle } from "@/components/reviews/ReviewReplySheet";
import { ImageViewer, type ViewerImage } from "@/components/products/ImageViewer";
import { useDockClearance } from "@/components/navigation/dock-metrics";

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString("en-AU", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

export default function ReviewDetailScreen() {
  const dockPad = useDockClearance();
  const { id } = useLocalSearchParams<{ id: string }>();
  const { data: review, isLoading, error } = useReview(id);

  const approve = useApproveReview();
  const reject = useRejectReview();
  const toggleFeatured = useToggleReviewFeatured();
  const reply = useReplyToReview();
  const replySheetRef = useRef<ReviewReplySheetHandle>(null);
  const [viewerImage, setViewerImage] = useState<ViewerImage | null>(null);

  const isMutating =
    approve.isPending || reject.isPending || toggleFeatured.isPending || reply.isPending;

  const handleReplySubmit = useCallback(
    (content: string) => {
      if (!review) return;
      reply.mutate({ id: review.id, content });
    },
    [review, reply],
  );

  if (error) {
    return (
      <Screen>
        <BackHeader eyebrow="REVIEW" />
        <View style={styles.centered}>
          <Text preset="h3" color="danger">
            Failed to load review
          </Text>
        </View>
      </Screen>
    );
  }

  if (isLoading || !review) {
    return (
      <Screen>
        <BackHeader eyebrow="REVIEW" title="Loading…" />
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      </Screen>
    );
  }

  return (
    <Screen>
      <BackHeader eyebrow="REVIEW" title={review.customer_name} />
      <ScrollView contentContainerStyle={[styles.scroll, { paddingBottom: dockPad }]}>
        <View style={styles.header}>
          <View style={styles.metaRow}>
            <ReviewStars rating={review.rating} size={16} />
            <ReviewStatusBadge status={review.status} />
          </View>
          <View style={styles.chips}>
            {review.verified_purchase ? <StatusBadge label="Verified purchase" tone="muted" /> : null}
            {review.featured ? <StatusBadge label="Featured" tone="info" /> : null}
          </View>

          {review.title ? (
            <Text preset="h3" color="text" style={styles.title}>
              {review.title}
            </Text>
          ) : null}
          <Text preset="body" color="text" style={styles.body}>
            {review.content}
          </Text>
          <Text preset="caption" color="textTertiary" style={styles.byline}>
            {review.customer_name} · {review.customer_email} · {formatDate(review.created_at)}
          </Text>
        </View>

        {review.media.length > 0 ? (
          <View style={styles.section}>
            <Eyebrow label="Photos" />
            <ScrollView horizontal showsHorizontalScrollIndicator={false} style={styles.mediaRow}>
              {review.media.map((m) => {
                const isVideo = m.media_type === "video";
                return (
                  <Pressable
                    key={m.id}
                    onPress={() => (isVideo ? undefined : setViewerImage({ uri: m.url }))}
                    accessibilityRole={isVideo ? "image" : "button"}
                    accessibilityLabel={isVideo ? "Review video" : "View review photo"}
                  >
                    <Image source={{ uri: m.url }} style={styles.thumb} />
                    {isVideo ? (
                      <View style={styles.videoBadge}>
                        <Text preset="caption" color="inverse">
                          Video
                        </Text>
                      </View>
                    ) : null}
                  </Pressable>
                );
              })}
            </ScrollView>
          </View>
        ) : null}

        {review.replies.length > 0 ? (
          <View style={styles.section}>
            <Eyebrow label="Replies" />
            <View style={styles.replyList}>
              {review.replies.map((r, i) => (
                <View key={r.id}>
                  {i > 0 ? <Hairline style={styles.replyDivider} /> : null}
                  <View style={styles.reply}>
                    <View style={styles.replyHead}>
                      <Text preset="bodyEmphasis" color="text">
                        {r.author_name}
                      </Text>
                      <Text preset="caption" color="textTertiary">
                        {r.author_type === "merchant" ? "You" : "Customer"} · {formatDate(r.created_at)}
                      </Text>
                    </View>
                    <Text preset="body" color="textSecondary">
                      {r.content}
                    </Text>
                  </View>
                </View>
              ))}
            </View>
          </View>
        ) : null}

        <View style={styles.actions}>
          {review.status !== "approved" ? (
            <Pressable
              onPress={() => approve.mutate(review.id)}
              disabled={isMutating}
              accessibilityRole="button"
              accessibilityLabel="Approve review"
              android_ripple={{ color: "rgba(247, 246, 242, 0.24)" }}
              style={({ pressed }) => [
                styles.approveBtn,
                pressed && Platform.OS === "ios" ? { opacity: 0.85 } : null,
              ]}
            >
              <Text preset="bodyEmphasis" color="inverse">
                {approve.isPending ? "Approving…" : "Approve"}
              </Text>
            </Pressable>
          ) : null}

          <Pressable
            onPress={() => replySheetRef.current?.present()}
            disabled={isMutating}
            accessibilityRole="button"
            accessibilityLabel="Reply to review"
            android_ripple={{ color: "rgba(14, 14, 12, 0.12)" }}
            style={({ pressed }) => [
              styles.replyBtn,
              pressed && Platform.OS === "ios" ? { opacity: 0.55 } : null,
            ]}
          >
            <Text preset="bodyEmphasis" color="text">
              Reply
            </Text>
          </Pressable>

          <View style={styles.actionRow}>
            {review.status !== "rejected" ? (
              <Pressable
                onPress={() => reject.mutate(review.id)}
                disabled={isMutating}
                accessibilityRole="button"
                accessibilityLabel="Reject review"
                android_ripple={{ color: "rgba(139, 46, 32, 0.12)" }}
                style={({ pressed }) => [
                  styles.rejectBtn,
                  pressed && Platform.OS === "ios" ? { opacity: 0.55 } : null,
                ]}
              >
                <Text preset="bodyEmphasis" color="danger">
                  {reject.isPending ? "Rejecting…" : "Reject"}
                </Text>
              </Pressable>
            ) : null}
            <Pressable
              onPress={() => toggleFeatured.mutate({ id: review.id, featured: !review.featured })}
              disabled={isMutating}
              accessibilityRole="button"
              accessibilityLabel={review.featured ? "Unfeature review" : "Feature review"}
              android_ripple={{ color: "rgba(14, 14, 12, 0.12)" }}
              style={({ pressed }) => [
                styles.featureBtn,
                pressed && Platform.OS === "ios" ? { opacity: 0.55 } : null,
              ]}
            >
              <Text preset="bodyEmphasis" color="text">
                {review.featured ? "Unfeature" : "Feature"}
              </Text>
            </Pressable>
          </View>
        </View>
      </ScrollView>

      <ReviewReplySheet
        ref={replySheetRef}
        onSubmit={handleReplySubmit}
        isSubmitting={reply.isPending}
      />
      <ImageViewer image={viewerImage} onClose={() => setViewerImage(null)} />
    </Screen>
  );
}

const styles = StyleSheet.create({
  scroll: { paddingBottom: theme.spacing.huge },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
  header: {
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.xl,
    gap: theme.spacing.sm,
  },
  metaRow: { flexDirection: "row", alignItems: "center", gap: theme.spacing.md },
  chips: { flexDirection: "row", gap: theme.spacing.xs, flexWrap: "wrap" },
  title: { marginTop: theme.spacing.xs },
  body: { marginTop: theme.spacing.xs },
  byline: { marginTop: theme.spacing.sm },
  section: {
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.xl,
  },
  mediaRow: { marginTop: theme.spacing.sm },
  thumb: {
    width: 96,
    height: 96,
    borderRadius: theme.radii.md,
    marginRight: theme.spacing.sm,
    backgroundColor: theme.colors.surfaceAlt,
  },
  videoBadge: {
    position: "absolute",
    bottom: theme.spacing.xs + 2,
    left: theme.spacing.xs,
    backgroundColor: theme.colors.text,
    paddingHorizontal: 6,
    paddingVertical: 2,
    borderRadius: 4,
  },
  replyList: { marginTop: theme.spacing.sm },
  reply: { paddingVertical: theme.spacing.md, gap: 4 },
  replyHead: { flexDirection: "row", alignItems: "baseline", justifyContent: "space-between", gap: theme.spacing.sm },
  replyDivider: { marginVertical: 0 },
  actions: {
    paddingHorizontal: theme.spacing.lg,
    paddingTop: theme.spacing.xl,
    gap: theme.spacing.md,
  },
  actionRow: { flexDirection: "row", gap: theme.spacing.md },
  approveBtn: {
    backgroundColor: theme.colors.accent,
    height: 48,
    borderRadius: theme.radii.md,
    alignItems: "center",
    justifyContent: "center",
  },
  replyBtn: {
    borderWidth: 1,
    borderColor: theme.colors.border,
    height: 48,
    borderRadius: theme.radii.md,
    alignItems: "center",
    justifyContent: "center",
  },
  rejectBtn: {
    flex: 1,
    borderWidth: 1,
    borderColor: theme.colors.danger,
    height: 48,
    borderRadius: theme.radii.md,
    alignItems: "center",
    justifyContent: "center",
  },
  featureBtn: {
    flex: 1,
    borderWidth: 1,
    borderColor: theme.colors.border,
    height: 48,
    borderRadius: theme.radii.md,
    alignItems: "center",
    justifyContent: "center",
  },
});
