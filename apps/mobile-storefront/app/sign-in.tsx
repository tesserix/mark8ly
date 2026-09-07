import { View, StyleSheet } from "react-native";
import { Stack, useRouter } from "expo-router";
import { Button, EmptyState, Screen } from "@/components/ui";
import { theme } from "@/lib/theme";

/**
 * Customer sign-in is not available in this build.
 *
 * The hosted identity pool this screen used to authenticate
 * against was removed (#787, #792), and no customer auth endpoint has been
 * built on its replacement yet. Rather than render a login form whose submit
 * throws an opaque error, this screen says so plainly and keeps the rest of
 * the app — browse, cart, checkout as a guest — reachable.
 *
 * When a customer auth flow exists, this screen becomes the form again.
 */
export default function SignInScreen() {
  const router = useRouter();

  return (
    <Screen>
      <Stack.Screen options={{ headerShown: false }} />
      <View style={styles.center}>
        <EmptyState
          title="Sign-in unavailable"
          message="Accounts aren't available in this version of the app yet. You can still browse and check out as a guest."
          action={
            <Button
              label="Continue browsing"
              onPress={() => router.replace("/(tabs)")}
              fullWidth
            />
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
