import { useCallback, useState } from "react";
import {
  Platform,
  View,
  ScrollView,
  Pressable,
  Alert,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useLocalSearchParams, useRouter } from "expo-router";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { ApiError } from "@repo/mobile-shared/api/client";
import { useCampaign } from "@/lib/hooks/use-campaigns";
import {
  useSendCampaign,
  usePauseCampaign,
  useResumeCampaign,
  useDeleteCampaign,
} from "@/lib/admin-api/campaign-actions";
import { BackHeader, Eyebrow, Hairline, Screen, StatusBadge, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { useDockClearance } from "@/components/navigation/dock-metrics";
import { formatMoney } from "@/lib/money";
import { campaignStatusTone, titleizeStatus } from "@/lib/campaign-display";

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <View style={styles.metric}>
      <Text preset="h3" color="text">
        {value}
      </Text>
      <Text preset="caption" color="textTertiary">
        {label}
      </Text>
    </View>
  );
}

export default function CampaignDetailScreen() {
  const router = useRouter();
  const { id } = useLocalSearchParams<{ id: string }>();
  const currency = useTenantStore((s) => s.activeStore?.currency_code) || "AUD";
  const { data: campaign, isLoading, error } = useCampaign(id);
  const dockPad = useDockClearance();

  const send = useSendCampaign();
  const pause = usePauseCampaign();
  const resume = useResumeCampaign();
  const del = useDeleteCampaign();
  const busy = send.isPending || pause.isPending || resume.isPending || del.isPending;

  const alertErr = (title: string) => (err: unknown) =>
    Alert.alert(
      title,
      err instanceof ApiError && err.status === 429
        ? "The send queue is busy or the daily budget is reached. Try again later."
        : err instanceof ApiError
          ? err.message
          : "Please try again.",
    );

  const onSend = useCallback(() => {
    Alert.alert("Send campaign", "Send this campaign to its audience now?", [
      { text: "Cancel", style: "cancel" },
      { text: "Send", onPress: () => send.mutate(id, { onError: alertErr("Couldn't send") }) },
    ]);
  }, [id, send]);

  const onPause = useCallback(
    () => pause.mutate(id, { onError: alertErr("Couldn't pause") }),
    [id, pause],
  );
  const onResume = useCallback(
    () => resume.mutate(id, { onError: alertErr("Couldn't resume") }),
    [id, resume],
  );

  const onDelete = useCallback(() => {
    Alert.alert("Delete campaign", "This permanently removes the campaign.", [
      { text: "Cancel", style: "cancel" },
      {
        text: "Delete",
        style: "destructive",
        onPress: () =>
          del.mutate(id, {
            onSuccess: () => router.back(),
            onError: alertErr("Couldn't delete"),
          }),
      },
    ]);
  }, [id, del, router]);

  if (error) {
    return (
      <Screen>
        <BackHeader eyebrow="CAMPAIGN" />
        <View style={styles.centered}>
          <Text preset="h3" color="danger">
            Failed to load campaign
          </Text>
        </View>
      </Screen>
    );
  }

  if (isLoading || !campaign) {
    return (
      <Screen>
        <BackHeader eyebrow="CAMPAIGN" title="Loading…" />
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      </Screen>
    );
  }

  const canSend = campaign.status === "draft" || campaign.status === "scheduled";
  const canPause = campaign.status === "sending" || campaign.status === "scheduled";
  const canResume = campaign.status === "paused";
  const canDelete = campaign.status !== "sent" && campaign.status !== "sending";
  const showMetrics = campaign.status !== "draft";

  return (
    <Screen>
      <BackHeader eyebrow="CAMPAIGN" title={campaign.name} />
      <ScrollView contentContainerStyle={[styles.scroll, { paddingBottom: dockPad }]}>
        <View style={styles.heading}>
          <Text preset="h1" color="text">
            {campaign.name}
          </Text>
          <StatusBadge label={titleizeStatus(campaign.status)} tone={campaignStatusTone(campaign.status)} />
          {campaign.subject ? (
            <Text preset="body" color="textSecondary">
              {campaign.subject}
            </Text>
          ) : null}
        </View>

        {showMetrics ? (
          <>
            <Eyebrow label="Performance" style={styles.section} />
            <View style={styles.metricsCard}>
              <Metric label="Recipients" value={String(campaign.total_recipients)} />
              <Metric label="Delivered" value={String(campaign.delivered)} />
              <Metric label="Opened" value={String(campaign.opened)} />
              <Metric label="Clicked" value={String(campaign.clicked)} />
              <Metric label="Converted" value={String(campaign.converted)} />
              <Metric label="Revenue" value={formatMoney(campaign.revenue, currency)} />
            </View>
          </>
        ) : null}

        {campaign.content ? (
          <>
            <Eyebrow label="Content" style={styles.section} />
            <View style={styles.card}>
              <Text preset="body" color="textSecondary">
                {campaign.content}
              </Text>
            </View>
          </>
        ) : null}

        <View style={styles.actions}>
          {canSend ? (
            <ActionButton
              variant="primary"
              label={send.isPending ? "Sending…" : "Send now"}
              onPress={onSend}
              disabled={busy}
            />
          ) : null}
          {canPause ? (
            <ActionButton
              variant="secondary"
              label={pause.isPending ? "Pausing…" : "Pause"}
              onPress={onPause}
              disabled={busy}
            />
          ) : null}
          {canResume ? (
            <ActionButton
              variant="secondary"
              label={resume.isPending ? "Resuming…" : "Resume"}
              onPress={onResume}
              disabled={busy}
            />
          ) : null}
          {canDelete ? (
            <ActionButton
              variant="danger"
              label={del.isPending ? "Deleting…" : "Delete"}
              onPress={onDelete}
              disabled={busy}
            />
          ) : null}
        </View>

        <Hairline />
        <Text preset="caption" color="textTertiary" style={styles.note}>
          Scheduling and audience targeting are managed from the web dashboard.
        </Text>
      </ScrollView>
    </Screen>
  );
}

function ActionButton({
  variant,
  label,
  onPress,
  disabled,
}: {
  variant: "primary" | "secondary" | "danger";
  label: string;
  onPress: () => void;
  disabled?: boolean;
}) {
  const btnStyle =
    variant === "primary" ? styles.btnPrimary : variant === "danger" ? styles.btnDanger : styles.btnSecondary;
  const color = variant === "primary" ? "inverse" : variant === "danger" ? "danger" : "text";
  // Primary is a solid moss fill (light text on it); secondary/danger are
  // outline-only on Paper. Tune the press feedback so it reads on each.
  const ripple =
    variant === "primary"
      ? theme.press.rippleOnDark
      : variant === "danger"
        ? theme.press.rippleDanger
        : theme.press.rippleInk;
  const pressedOpacity =
    variant === "primary" ? theme.press.opacitySolidFill : theme.press.opacityStandard;
  // NativeWind's JSX interop doesn't resolve a function `style` prop the way
  // it resolves a plain array — press state is tracked explicitly instead.
  const [pressed, setPressed] = useState(false);
  return (
    <Pressable
      onPress={onPress}
      onPressIn={() => setPressed(true)}
      onPressOut={() => setPressed(false)}
      disabled={disabled}
      accessibilityRole="button"
      accessibilityLabel={label}
      android_ripple={ripple}
      style={[
        btnStyle,
        disabled && styles.btnDisabled,
        pressed && Platform.OS === "ios" ? { opacity: pressedOpacity } : null,
      ]}
    >
      <Text preset="bodyEmphasis" color={color}>
        {label}
      </Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  scroll: { paddingBottom: theme.spacing.huge },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
  heading: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.lg, gap: theme.spacing.sm },
  section: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.xl },
  card: { paddingHorizontal: theme.spacing.lg, marginTop: theme.spacing.sm },
  metricsCard: {
    flexDirection: "row",
    flexWrap: "wrap",
    paddingHorizontal: theme.spacing.lg,
    marginTop: theme.spacing.sm,
    rowGap: theme.spacing.lg,
  },
  metric: { width: "33%", gap: 2 },
  actions: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.xl, gap: theme.spacing.sm },
  note: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.md },
  btnPrimary: {
    backgroundColor: theme.colors.accent,
    height: 48,
    borderRadius: theme.radii.md,
    alignItems: "center",
    justifyContent: "center",
  },
  btnSecondary: {
    borderWidth: 1,
    borderColor: theme.colors.border,
    height: 48,
    borderRadius: theme.radii.md,
    alignItems: "center",
    justifyContent: "center",
  },
  btnDanger: {
    borderWidth: 1,
    borderColor: theme.colors.danger,
    height: 48,
    borderRadius: theme.radii.md,
    alignItems: "center",
    justifyContent: "center",
  },
  btnDisabled: { opacity: 0.5 },
});
