// SupportChatView — the shared, themeable support-chat UI. It renders the
// intake form, the live message thread, the composer, and the closed/queue
// states from a UseSupportChat instance. Both mobile apps render it with
// their own palette so there is one chat UI, not two.
import { useMemo, useRef, useState } from "react";
import {
  ActivityIndicator,
  FlatList,
  KeyboardAvoidingView,
  Platform,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from "react-native";

import type { UseSupportChat } from "./useSupportChat";
import type { IntakeReason, SupportMessage } from "./types";

export interface SupportPalette {
  background: string;
  surface: string;
  bubbleOwn: string;
  textOnOwn: string;
  text: string;
  textSecondary: string;
  border: string;
  primary: string;
  onPrimary: string;
  danger: string;
}

export interface SupportChatViewProps {
  chat: UseSupportChat;
  palette: SupportPalette;
  reasons: IntakeReason[];
  defaults?: { name?: string; email?: string };
  introTitle?: string;
  introSubtitle?: string;
  composerPlaceholder?: string;
  statusPlaceholder?: string;
  /** Returns true when the selected reason requires a date of birth. */
  requiresDob?: (reason: string) => boolean;
}

export function SupportChatView({
  chat,
  palette,
  reasons,
  defaults,
  introTitle = "How can we help?",
  introSubtitle = "Start a conversation and we'll get back to you.",
  composerPlaceholder = "Type a message…",
  statusPlaceholder = "Briefly, what's going on?",
  requiresDob,
}: SupportChatViewProps) {
  const [composeNew, setComposeNew] = useState(false);

  const showIntake = composeNew || (!chat.conversation && chat.status !== "loading");

  if (chat.status === "loading" && !chat.conversation && !composeNew) {
    return (
      <View style={[styles.center, { backgroundColor: palette.background }]}>
        <ActivityIndicator color={palette.primary} />
      </View>
    );
  }

  if (showIntake) {
    return (
      <IntakeForm
        palette={palette}
        reasons={reasons}
        defaults={defaults}
        introTitle={introTitle}
        introSubtitle={introSubtitle}
        statusPlaceholder={statusPlaceholder}
        requiresDob={requiresDob}
        submitting={chat.status === "loading"}
        error={chat.error}
        onSubmit={async (input) => {
          await chat.startConversation(input);
          setComposeNew(false);
        }}
      />
    );
  }

  return (
    <ThreadView
      chat={chat}
      palette={palette}
      composerPlaceholder={composerPlaceholder}
      onNewChat={() => setComposeNew(true)}
    />
  );
}

function ThreadView({
  chat,
  palette,
  composerPlaceholder,
  onNewChat,
}: {
  chat: UseSupportChat;
  palette: SupportPalette;
  composerPlaceholder: string;
  onNewChat: () => void;
}) {
  const [draft, setDraft] = useState("");
  const [sending, setSending] = useState(false);
  const listRef = useRef<FlatList<SupportMessage>>(null);
  const closed = chat.conversation?.status === "closed" || chat.status === "closed";

  const send = async () => {
    const body = draft.trim();
    if (!body || sending) return;
    setSending(true);
    setDraft("");
    try {
      await chat.sendMessage(body);
    } catch {
      setDraft(body); // restore on failure
    } finally {
      setSending(false);
    }
  };

  return (
    <KeyboardAvoidingView
      style={[styles.flex, { backgroundColor: palette.background }]}
      behavior={Platform.OS === "ios" ? "padding" : undefined}
      keyboardVerticalOffset={Platform.OS === "ios" ? 90 : 0}
    >
      <StatusBar chat={chat} palette={palette} />
      <FlatList
        ref={listRef}
        data={chat.messages}
        keyExtractor={(m) => m.id}
        renderItem={({ item }) => <MessageBubble message={item} palette={palette} />}
        contentContainerStyle={styles.listContent}
        onContentSizeChange={() => listRef.current?.scrollToEnd({ animated: true })}
        showsVerticalScrollIndicator={false}
      />
      {closed ? (
        <View style={[styles.closedBar, { borderTopColor: palette.border }]}>
          <Text style={[styles.closedText, { color: palette.textSecondary }]}>
            This conversation is closed.
          </Text>
          <Pressable onPress={onNewChat} accessibilityRole="button" accessibilityLabel="Start a new chat">
            <Text style={[styles.link, { color: palette.primary }]}>Start a new chat</Text>
          </Pressable>
        </View>
      ) : (
        <View style={[styles.composer, { borderTopColor: palette.border, backgroundColor: palette.background }]}>
          <TextInput
            style={[styles.input, { backgroundColor: palette.surface, color: palette.text, borderColor: palette.border }]}
            placeholder={composerPlaceholder}
            placeholderTextColor={palette.textSecondary}
            value={draft}
            onChangeText={setDraft}
            multiline
            accessibilityLabel="Message"
          />
          <Pressable
            style={[styles.sendBtn, { backgroundColor: draft.trim() ? palette.primary : palette.border }]}
            onPress={send}
            disabled={!draft.trim() || sending}
            accessibilityRole="button"
            accessibilityLabel="Send message"
          >
            <Text style={[styles.sendText, { color: palette.onPrimary }]}>{sending ? "…" : "Send"}</Text>
          </Pressable>
        </View>
      )}
    </KeyboardAvoidingView>
  );
}

function StatusBar({ chat, palette }: { chat: UseSupportChat; palette: SupportPalette }) {
  const conv = chat.conversation;
  const label = useMemo(() => {
    if (chat.status === "closed" || conv?.status === "closed") return "Closed";
    if (conv?.status === "pending") return chat.connected ? "Waiting for an agent…" : "Connecting…";
    if (conv?.status === "active") return chat.connected ? "Connected" : "Reconnecting…";
    return "";
  }, [chat.status, chat.connected, conv?.status]);

  if (!conv) return null;
  return (
    <View style={[styles.statusBar, { borderBottomColor: palette.border }]}>
      <View style={[styles.dot, { backgroundColor: chat.connected ? "#1f9d55" : palette.textSecondary }]} />
      <Text style={[styles.statusText, { color: palette.textSecondary }]}>
        {label}
        {conv.case_id ? `  ·  ${conv.case_id}` : ""}
      </Text>
    </View>
  );
}

function MessageBubble({ message, palette }: { message: SupportMessage; palette: SupportPalette }) {
  if (message.sender_type === "system") {
    return (
      <View style={styles.systemRow}>
        <Text style={[styles.systemText, { color: palette.textSecondary }]}>{message.body}</Text>
      </View>
    );
  }
  const own = message.sender_type === "customer";
  return (
    <View style={[styles.bubbleRow, own ? styles.rowEnd : styles.rowStart]}>
      <View
        style={[
          styles.bubble,
          own
            ? { backgroundColor: palette.bubbleOwn }
            : { backgroundColor: palette.surface, borderColor: palette.border, borderWidth: StyleSheet.hairlineWidth },
        ]}
      >
        {!own && message.sender_name ? (
          <Text style={[styles.senderName, { color: palette.textSecondary }]}>{message.sender_name}</Text>
        ) : null}
        <Text style={[styles.bubbleText, { color: own ? palette.textOnOwn : palette.text }]}>{message.body}</Text>
      </View>
    </View>
  );
}

function IntakeForm({
  palette,
  reasons,
  defaults,
  introTitle,
  introSubtitle,
  statusPlaceholder,
  requiresDob,
  submitting,
  error,
  onSubmit,
}: {
  palette: SupportPalette;
  reasons: IntakeReason[];
  defaults?: { name?: string; email?: string };
  introTitle: string;
  introSubtitle: string;
  statusPlaceholder: string;
  requiresDob?: (reason: string) => boolean;
  submitting: boolean;
  error: string | null;
  onSubmit: (input: {
    message: string;
    reason: string;
    statusInfo: string;
    name?: string;
    email?: string;
    dob?: string;
  }) => Promise<void>;
}) {
  const [reason, setReason] = useState(reasons[0]?.value ?? "other");
  const [statusInfo, setStatusInfo] = useState("");
  const [message, setMessage] = useState("");
  const [dob, setDob] = useState("");

  const selected = reasons.find((r) => r.value === reason);
  const needsStatus = selected?.requiresStatus !== false;
  const needsDob = requiresDob?.(reason) ?? false;
  const canSubmit =
    message.trim().length > 0 && (!needsStatus || statusInfo.trim().length > 0) && (!needsDob || dob.trim().length > 0);

  return (
    <KeyboardAvoidingView
      style={[styles.flex, { backgroundColor: palette.background }]}
      behavior={Platform.OS === "ios" ? "padding" : undefined}
    >
      <ScrollView keyboardShouldPersistTaps="handled" showsVerticalScrollIndicator={false}>
        <View style={styles.intake}>
            <Text style={[styles.introTitle, { color: palette.text }]}>{introTitle}</Text>
            <Text style={[styles.introSubtitle, { color: palette.textSecondary }]}>{introSubtitle}</Text>

            <Text style={[styles.label, { color: palette.text }]}>Topic</Text>
            <View style={styles.chips}>
              {reasons.map((r) => {
                const active = r.value === reason;
                return (
                  <Pressable
                    key={r.value}
                    onPress={() => setReason(r.value)}
                    style={[
                      styles.chip,
                      {
                        backgroundColor: active ? palette.primary : palette.surface,
                        borderColor: active ? palette.primary : palette.border,
                      },
                    ]}
                    accessibilityRole="button"
                    accessibilityState={{ selected: active }}
                    accessibilityLabel={r.label}
                  >
                    <Text style={{ color: active ? palette.onPrimary : palette.text, fontSize: 13 }}>{r.label}</Text>
                  </Pressable>
                );
              })}
            </View>

            {needsStatus ? (
              <>
                <Text style={[styles.label, { color: palette.text }]}>Summary</Text>
                <TextInput
                  style={[styles.field, { backgroundColor: palette.surface, color: palette.text, borderColor: palette.border }]}
                  placeholder={statusPlaceholder}
                  placeholderTextColor={palette.textSecondary}
                  value={statusInfo}
                  onChangeText={setStatusInfo}
                  accessibilityLabel="Summary"
                />
              </>
            ) : null}

            {needsDob ? (
              <>
                <Text style={[styles.label, { color: palette.text }]}>Date of birth (YYYY-MM-DD)</Text>
                <TextInput
                  style={[styles.field, { backgroundColor: palette.surface, color: palette.text, borderColor: palette.border }]}
                  placeholder="1990-01-01"
                  placeholderTextColor={palette.textSecondary}
                  value={dob}
                  onChangeText={setDob}
                  autoCapitalize="none"
                  accessibilityLabel="Date of birth"
                />
              </>
            ) : null}

            <Text style={[styles.label, { color: palette.text }]}>Message</Text>
            <TextInput
              style={[styles.fieldMultiline, { backgroundColor: palette.surface, color: palette.text, borderColor: palette.border }]}
              placeholder="Describe your issue…"
              placeholderTextColor={palette.textSecondary}
              value={message}
              onChangeText={setMessage}
              multiline
              accessibilityLabel="Message"
            />

            {error ? <Text style={[styles.error, { color: palette.danger }]}>{error}</Text> : null}

            <Pressable
              style={[styles.submit, { backgroundColor: canSubmit ? palette.primary : palette.border }]}
              disabled={!canSubmit || submitting}
              onPress={() => onSubmit({ message: message.trim(), reason, statusInfo: statusInfo.trim(), name: defaults?.name, email: defaults?.email, dob: dob.trim() || undefined })}
              accessibilityRole="button"
              accessibilityLabel="Start chat"
            >
              <Text style={[styles.submitText, { color: palette.onPrimary }]}>{submitting ? "Starting…" : "Start chat"}</Text>
            </Pressable>
        </View>
      </ScrollView>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  flex: { flex: 1 },
  center: { flex: 1, alignItems: "center", justifyContent: "center" },
  listContent: { padding: 14, gap: 8 },
  statusBar: { flexDirection: "row", alignItems: "center", gap: 8, paddingHorizontal: 14, paddingVertical: 8, borderBottomWidth: StyleSheet.hairlineWidth },
  dot: { width: 8, height: 8, borderRadius: 4 },
  statusText: { fontSize: 12 },
  bubbleRow: { flexDirection: "row" },
  rowEnd: { justifyContent: "flex-end" },
  rowStart: { justifyContent: "flex-start" },
  bubble: { maxWidth: "82%", borderRadius: 14, paddingHorizontal: 12, paddingVertical: 8 },
  senderName: { fontSize: 11, marginBottom: 2, fontWeight: "600" },
  bubbleText: { fontSize: 15, lineHeight: 20 },
  systemRow: { alignItems: "center", paddingVertical: 4 },
  systemText: { fontSize: 12, fontStyle: "italic" },
  composer: { flexDirection: "row", alignItems: "flex-end", gap: 8, padding: 10, borderTopWidth: StyleSheet.hairlineWidth },
  input: { flex: 1, minHeight: 40, maxHeight: 120, borderRadius: 10, borderWidth: StyleSheet.hairlineWidth, paddingHorizontal: 12, paddingTop: 10, paddingBottom: 10, fontSize: 15 },
  sendBtn: { height: 40, paddingHorizontal: 16, borderRadius: 10, alignItems: "center", justifyContent: "center" },
  sendText: { fontSize: 15, fontWeight: "600" },
  closedBar: { padding: 16, borderTopWidth: StyleSheet.hairlineWidth, alignItems: "center", gap: 6 },
  closedText: { fontSize: 14 },
  link: { fontSize: 15, fontWeight: "600" },
  intake: { padding: 20, gap: 10 },
  introTitle: { fontSize: 22, fontWeight: "700" },
  introSubtitle: { fontSize: 14, lineHeight: 20, marginBottom: 6 },
  label: { fontSize: 13, fontWeight: "600", marginTop: 8 },
  chips: { flexDirection: "row", flexWrap: "wrap", gap: 8 },
  chip: { paddingHorizontal: 12, paddingVertical: 8, borderRadius: 16, borderWidth: StyleSheet.hairlineWidth },
  field: { height: 44, borderRadius: 10, borderWidth: StyleSheet.hairlineWidth, paddingHorizontal: 12, fontSize: 15 },
  fieldMultiline: { minHeight: 90, borderRadius: 10, borderWidth: StyleSheet.hairlineWidth, paddingHorizontal: 12, paddingTop: 10, fontSize: 15, textAlignVertical: "top" },
  error: { fontSize: 13, marginTop: 4 },
  submit: { height: 50, borderRadius: 10, alignItems: "center", justifyContent: "center", marginTop: 14 },
  submitText: { fontSize: 16, fontWeight: "700" },
});
