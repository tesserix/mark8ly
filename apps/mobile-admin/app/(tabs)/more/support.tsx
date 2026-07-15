// Merchant -> platform support chat (#119). Renders the shared
// SupportChatView wired to the marketplace-api admin platform-support BFF,
// which bridges to otto's "platform" tenant. A Tesserix employee handles
// the conversation from the platform-support inbox; the merchant's own
// tenant is attached server-side so they know who's asking.
import { useMemo } from "react";
import { View, StyleSheet } from "react-native";
import * as SecureStore from "expo-secure-store";

import {
  createSupportClient,
  useSupportChat,
  SupportChatView,
  type IntakeReason,
  type SupportPalette,
} from "@repo/mobile-shared/support";
import { secureStoreKV } from "@repo/mobile-shared/support/storage";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { useAuthNoticeStore } from "@repo/mobile-shared/stores/auth-notice";
import { useEnvironment } from "@repo/mobile-shared/config/env";

import { BackHeader, Screen } from "@/components/ui";
import { theme } from "@/lib/theme";

const REASONS: IntakeReason[] = [
  { value: "general_question", label: "Quick question", requiresStatus: false },
  { value: "billing", label: "Billing" },
  { value: "domains", label: "Domain / DNS" },
  { value: "payments_setup", label: "Payments setup" },
  { value: "incident", label: "Outage / incident" },
  { value: "kyc_compliance", label: "KYC / compliance" },
  { value: "feature_request", label: "Feature request" },
  { value: "other", label: "Something else" },
];

const SESSION_KEY = "otto_platform_support_session";

export default function PlatformSupportScreen() {
  const { user, getToken, refreshToken, signOut } = useAuth();
  const env = useEnvironment();

  const client = useMemo(
    () =>
      createSupportClient({
        baseUrl: env.apiBaseUrl,
        basePath: "/api/v1/mobile/admin/platform-support",
        getToken,
        refreshToken,
        onUnauthorized: async () => {
          // The support client can't distinguish reasons (unlike the main
          // API client's onUnauthorized(reason)) — "no-session" is the
          // honest value: don't invent an access-denied signal it never saw.
          useAuthNoticeStore.getState().setNotice("no-session");
          await signOut();
        },
        loadSessionToken: () => SecureStore.getItemAsync(SESSION_KEY),
        saveSessionToken: (t) => SecureStore.setItemAsync(SESSION_KEY, t),
      }),
    [env.apiBaseUrl, getToken, refreshToken, signOut],
  );

  const chat = useSupportChat({ client, storage: secureStoreKV });

  const palette = useMemo<SupportPalette>(
    () => ({
      background: theme.colors.background,
      surface: theme.colors.surfaceAlt,
      bubbleOwn: theme.colors.accent,
      textOnOwn: theme.colors.palette.white,
      text: theme.colors.text,
      textSecondary: theme.colors.textSecondary,
      border: theme.colors.border,
      primary: theme.colors.accent,
      onPrimary: theme.colors.palette.white,
      danger: theme.colors.danger,
    }),
    [],
  );

  return (
    <Screen>
      <BackHeader title="Tesserix Support" eyebrow="HELP" />
      <View style={styles.fill}>
        <SupportChatView
          chat={chat}
          palette={palette}
          reasons={REASONS}
          defaults={{ name: user?.displayName ?? undefined, email: user?.email ?? undefined }}
          introTitle="Chat with Tesserix"
          introSubtitle="Questions about billing, domains, payments, or an incident? We're here to help."
          composerPlaceholder="Type a message…"
          statusPlaceholder="e.g. Checkout returning 500 since 09:00 UTC"
        />
      </View>
    </Screen>
  );
}

const styles = StyleSheet.create({
  fill: { flex: 1 },
});
