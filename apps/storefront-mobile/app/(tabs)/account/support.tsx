// Customer support chat (#118) — customer -> merchant. Renders the shared
// SupportChatView wired to the marketplace-api mobile support BFF, which
// bridges to otto. The otto session token is persisted per-store so the
// thread resumes across app launches.
import { useMemo } from "react";
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

import { useTheme } from "@/lib/theme/theme-provider";
import { useApiBaseUrl, useStoreSlug } from "@/lib/storefront-api/client-provider";

const REASONS: IntakeReason[] = [
  { value: "order_issue", label: "Order issue" },
  { value: "return", label: "Return / refund" },
  { value: "payment", label: "Payment" },
  { value: "product_question", label: "Product question", requiresStatus: false },
  { value: "other", label: "Something else" },
];

// otto requires a date of birth for order/account-related reasons so staff
// can verify the customer before discussing an order.
const DOB_REASONS = new Set(["order_issue", "return", "payment"]);
const SESSION_PREFIX = "otto_support_session";

export default function SupportScreen() {
  const theme = useTheme();
  const { user, getToken, refreshToken, signOut } = useAuth();
  const baseUrl = useApiBaseUrl();
  const storeSlug = useStoreSlug();

  const client = useMemo(
    () =>
      createSupportClient({
        baseUrl,
        basePath: `/api/v1/mobile/storefront/stores/${storeSlug}/support`,
        getToken,
        refreshToken,
        onUnauthorized: signOut,
        loadSessionToken: () => SecureStore.getItemAsync(`${SESSION_PREFIX}:${storeSlug}`),
        saveSessionToken: (t) => SecureStore.setItemAsync(`${SESSION_PREFIX}:${storeSlug}`, t),
      }),
    [baseUrl, storeSlug, getToken, refreshToken, signOut],
  );

  const chat = useSupportChat({ client, storage: secureStoreKV });

  const palette = useMemo<SupportPalette>(
    () => ({
      background: theme.background,
      surface: theme.elevated,
      bubbleOwn: theme.primary,
      textOnOwn: theme.elevated,
      text: theme.text,
      textSecondary: theme.textSecondary,
      border: theme.border,
      primary: theme.primary,
      onPrimary: theme.elevated,
      danger: "#8B2020",
    }),
    [theme],
  );

  return (
    <SupportChatView
      chat={chat}
      palette={palette}
      reasons={REASONS}
      defaults={{ name: user?.displayName ?? undefined, email: user?.email ?? undefined }}
      requiresDob={(r) => DOB_REASONS.has(r)}
      introTitle="How can we help?"
      introSubtitle="Chat with the store's support team."
      composerPlaceholder="Type a message…"
      statusPlaceholder="e.g. Order #1234 hasn't arrived"
    />
  );
}
