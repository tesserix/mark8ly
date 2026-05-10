import { useEffect } from "react";
import { Linking, View, StyleSheet } from "react-native";
import { Stack, useRouter } from "expo-router";
import { Button, EmptyState, Screen } from "@/components/ui";
import { theme } from "@/lib/theme";
import { getMerchant } from "@/lib/merchant";

export default function SignUpScreen() {
  const router = useRouter();
  const merchant = getMerchant();
  const url = `https://${merchant.defaultStoreSlug}.mark8ly.com/create-account`;

  // Sign-up is web-only — the storefront site owns the full flow
  // (verify email, capture profile, etc.). Open it in the OS browser
  // immediately so the customer doesn't see a redundant intermediate.
  useEffect(() => {
    Linking.openURL(url).catch(() => {});
  }, [url]);

  return (
    <Screen>
      <Stack.Screen options={{ headerShown: false }} />
      <View style={styles.center}>
        <EmptyState
          title="Open in browser"
          message="Finish creating your account on the website, then come back to sign in."
          action={
            <View style={{ gap: theme.spacing.sm, alignSelf: "stretch" }}>
              <Button label="Open browser" onPress={() => Linking.openURL(url)} fullWidth />
              <Button
                label="Back to sign in"
                variant="secondary"
                onPress={() => router.replace("/sign-in")}
                fullWidth
              />
            </View>
          }
        />
      </View>
    </Screen>
  );
}

const styles = StyleSheet.create({
  center: {
    flex: 1,
    justifyContent: "center",
    paddingHorizontal: theme.spacing.lg,
  },
});
