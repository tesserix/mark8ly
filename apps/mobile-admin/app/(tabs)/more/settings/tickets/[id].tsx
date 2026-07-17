import { useCallback, useState } from "react";
import {
  View,
  ScrollView,
  Pressable,
  Alert,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { useLocalSearchParams } from "expo-router";
import { ApiError } from "@repo/mobile-shared/api/client";
import { useTicket } from "@/lib/hooks/use-tickets";
import { useReplyTicket, useUpdateTicketStatus } from "@/lib/admin-api/ticket-actions";
import { BackHeader, Eyebrow, FieldInput, Hairline, Screen, StatusBadge, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { TICKET_STATUS_TONE, titleizeStatus } from "./index";
import type { TicketReply } from "@repo/mobile-shared/api/types";

function formatWhen(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString("en-AU", { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
}

function Message({
  author,
  when,
  content,
  fromMerchant,
}: {
  author: string;
  when: string;
  content: string;
  fromMerchant: boolean;
}) {
  return (
    <View style={[styles.message, fromMerchant && styles.messageMerchant]}>
      <View style={styles.messageHead}>
        <Text preset="caption" color={fromMerchant ? "accent" : "textSecondary"}>
          {author}
        </Text>
        <Text preset="caption" color="textTertiary">
          {when}
        </Text>
      </View>
      <Text preset="body" color="text">
        {content}
      </Text>
    </View>
  );
}

export default function TicketDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { data: ticket, isLoading, error } = useTicket(id);
  const reply = useReplyTicket();
  const updateStatus = useUpdateTicketStatus();

  const [draft, setDraft] = useState("");

  const onSendReply = useCallback(() => {
    if (!draft.trim()) return;
    reply.mutate(
      { id, content: draft.trim() },
      {
        onSuccess: () => setDraft(""),
        onError: (err) =>
          Alert.alert("Couldn't send reply", err instanceof ApiError ? err.message : "Please try again."),
      },
    );
  }, [draft, id, reply]);

  const changeStatus = useCallback(
    (status: string) =>
      updateStatus.mutate(
        { id, status },
        {
          onError: (err) =>
            Alert.alert(
              "Couldn't update status",
              err instanceof ApiError ? err.message : "Please try again.",
            ),
        },
      ),
    [id, updateStatus],
  );

  if (error) {
    return (
      <Screen>
        <BackHeader eyebrow="TICKET" />
        <View style={styles.centered}>
          <Text preset="h3" color="danger">
            Failed to load ticket
          </Text>
        </View>
      </Screen>
    );
  }

  if (isLoading || !ticket) {
    return (
      <Screen>
        <BackHeader eyebrow="TICKET" title="Loading…" />
        <View style={styles.centered}>
          <ActivityIndicator size="small" color={theme.colors.text} />
        </View>
      </Screen>
    );
  }

  const isMerchant = (r: TicketReply) => r.author_type === "merchant" || r.author_type === "staff";
  const openish = ticket.status === "open" || ticket.status === "pending";

  return (
    <Screen>
      <BackHeader eyebrow="TICKET" title={`#${ticket.ticket_number}`} />
      <ScrollView contentContainerStyle={styles.scroll} keyboardShouldPersistTaps="handled">
        <View style={styles.heading}>
          <Text preset="h2" color="text">
            {ticket.subject}
          </Text>
          <StatusBadge label={titleizeStatus(ticket.status)} tone={TICKET_STATUS_TONE[ticket.status] ?? "muted"} />
        </View>

        <View style={styles.thread}>
          <Message
            author={ticket.submitted_by_name || ticket.submitted_by_email}
            when={formatWhen(ticket.created_at)}
            content={ticket.description}
            fromMerchant={false}
          />
          {ticket.replies.map((r) => (
            <Message
              key={r.id}
              author={r.author_name}
              when={formatWhen(r.created_at)}
              content={r.content}
              fromMerchant={isMerchant(r)}
            />
          ))}
        </View>

        <Eyebrow label="Reply" style={styles.section} />
        <View style={styles.replyBox}>
          <FieldInput
            value={draft}
            onChangeText={setDraft}
            placeholder="Write a reply…"
            multiline
            editable={!reply.isPending}
          />
          <Pressable
            style={[styles.sendBtn, (!draft.trim() || reply.isPending) && styles.disabled]}
            onPress={onSendReply}
            disabled={!draft.trim() || reply.isPending}
            accessibilityRole="button"
            accessibilityLabel="Send reply"
          >
            {reply.isPending ? (
              <ActivityIndicator size="small" color={theme.colors.inverse} />
            ) : (
              <Text preset="bodyEmphasis" color="inverse">
                Send reply
              </Text>
            )}
          </Pressable>
        </View>

        <Hairline />
        <View style={styles.statusActions}>
          {openish ? (
            <>
              <StatusButton label="Mark resolved" onPress={() => changeStatus("resolved")} disabled={updateStatus.isPending} />
              <StatusButton label="Close" onPress={() => changeStatus("closed")} disabled={updateStatus.isPending} />
            </>
          ) : (
            <StatusButton label="Reopen" onPress={() => changeStatus("open")} disabled={updateStatus.isPending} />
          )}
        </View>
      </ScrollView>
    </Screen>
  );
}

function StatusButton({ label, onPress, disabled }: { label: string; onPress: () => void; disabled?: boolean }) {
  return (
    <Pressable
      style={[styles.statusBtn, disabled && styles.disabled]}
      onPress={onPress}
      disabled={disabled}
      accessibilityRole="button"
      accessibilityLabel={label}
    >
      <Text preset="bodyEmphasis" color="text">
        {label}
      </Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  scroll: { padding: theme.spacing.lg, gap: theme.spacing.md, paddingBottom: theme.spacing.huge },
  centered: { flex: 1, alignItems: "center", justifyContent: "center" },
  heading: { gap: theme.spacing.sm },
  thread: { gap: theme.spacing.sm },
  message: {
    backgroundColor: theme.colors.surfaceAlt,
    borderRadius: theme.radii.md,
    padding: theme.spacing.md,
    gap: theme.spacing.xs,
  },
  messageMerchant: { backgroundColor: theme.colors.accentTint },
  messageHead: { flexDirection: "row", justifyContent: "space-between", gap: theme.spacing.sm },
  section: { paddingTop: theme.spacing.sm },
  replyBox: { gap: theme.spacing.sm },
  sendBtn: {
    height: 48,
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.text,
    alignItems: "center",
    justifyContent: "center",
  },
  statusActions: { flexDirection: "row", gap: theme.spacing.md },
  statusBtn: {
    flex: 1,
    height: 44,
    borderRadius: theme.radii.md,
    borderWidth: 1,
    borderColor: theme.colors.border,
    alignItems: "center",
    justifyContent: "center",
  },
  disabled: { opacity: 0.4 },
});
