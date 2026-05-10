import { useEffect, useState } from "react";
import {
  ActivityIndicator,
  Alert,
  KeyboardAvoidingView,
  Platform,
  ScrollView,
  StyleSheet,
  TextInput,
  TouchableOpacity,
  View,
} from "react-native";
import { Stack, useRouter } from "expo-router";
import { ChevronLeft, ChevronRight } from "lucide-react-native";
import { useProfile, useUpdateProfile } from "@/lib/hooks/use-account";
import { Button, Card, Hairline, PageHeader, Screen, Text } from "@/components/ui";
import { theme } from "@/lib/theme";

export default function ProfileScreen() {
  const router = useRouter();
  const { data: profile, isLoading } = useProfile();
  const update = useUpdateProfile();

  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  const [phone, setPhone] = useState("");

  useEffect(() => {
    if (!profile) return;
    setFirstName(profile.first_name ?? "");
    setLastName(profile.last_name ?? "");
    setPhone(profile.phone ?? "");
  }, [profile]);

  const dirty =
    !!profile &&
    (firstName !== (profile.first_name ?? "") ||
      lastName !== (profile.last_name ?? "") ||
      phone !== (profile.phone ?? ""));

  const handleSave = () => {
    update.mutate(
      { first_name: firstName, last_name: lastName, phone },
      {
        onSuccess: () =>
          Alert.alert("Saved", "Your profile has been updated."),
        onError: (err: unknown) =>
          Alert.alert(
            "Couldn't save",
            err instanceof Error ? err.message : "Try again in a moment.",
          ),
      },
    );
  };

  return (
    <Screen>
      <Stack.Screen options={{ headerShown: false }} />
      <KeyboardAvoidingView
        style={{ flex: 1 }}
        behavior={Platform.OS === "ios" ? "padding" : undefined}
      >
        <View style={styles.headerBar}>
          <TouchableOpacity
            onPress={() => router.back()}
            hitSlop={12}
            accessibilityRole="button"
            accessibilityLabel="Back"
            style={styles.backBtn}
          >
            <ChevronLeft size={22} color={theme.colors.text} strokeWidth={1.75} />
          </TouchableOpacity>
        </View>
        <ScrollView contentContainerStyle={styles.scroll} keyboardShouldPersistTaps="handled">
          <PageHeader eyebrow="ACCOUNT" title="Profile" subtitle={profile?.email ?? undefined} />

          {isLoading && !profile ? (
            <View style={styles.center}>
              <ActivityIndicator size="small" color={theme.colors.text} />
            </View>
          ) : (
            <>
              <Card padding="md" style={styles.card}>
                <Field label="First name">
                  <TextInput
                    value={firstName}
                    onChangeText={setFirstName}
                    style={styles.input}
                    placeholder="Jane"
                    placeholderTextColor={theme.colors.textTertiary}
                    autoComplete="given-name"
                    accessibilityLabel="First name"
                  />
                </Field>
                <Hairline style={{ marginVertical: theme.spacing.sm }} />
                <Field label="Last name">
                  <TextInput
                    value={lastName}
                    onChangeText={setLastName}
                    style={styles.input}
                    placeholder="Doe"
                    placeholderTextColor={theme.colors.textTertiary}
                    autoComplete="family-name"
                    accessibilityLabel="Last name"
                  />
                </Field>
                <Hairline style={{ marginVertical: theme.spacing.sm }} />
                <Field label="Phone">
                  <TextInput
                    value={phone}
                    onChangeText={setPhone}
                    style={styles.input}
                    placeholder="+1 555 123 4567"
                    placeholderTextColor={theme.colors.textTertiary}
                    keyboardType="phone-pad"
                    autoComplete="tel"
                    accessibilityLabel="Phone"
                  />
                </Field>
              </Card>

              <View style={styles.actions}>
                <Button
                  label="Save changes"
                  onPress={handleSave}
                  loading={update.isPending}
                  disabled={!dirty}
                  fullWidth
                />
              </View>

              <Card padding={0} style={styles.card}>
                <TouchableOpacity
                  onPress={() => router.push("/addresses")}
                  activeOpacity={0.6}
                  style={styles.row}
                  accessibilityRole="button"
                  accessibilityLabel="Saved addresses"
                >
                  <Text preset="bodyEmphasis" color="text" style={{ flex: 1 }}>
                    Saved addresses
                  </Text>
                  <ChevronRight size={16} color={theme.colors.textTertiary} strokeWidth={1.75} />
                </TouchableOpacity>
              </Card>
            </>
          )}
        </ScrollView>
      </KeyboardAvoidingView>
    </Screen>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <View style={{ gap: theme.spacing.xs }}>
      <Text preset="caption" color="textTertiary">
        {label.toUpperCase()}
      </Text>
      {children}
    </View>
  );
}

const styles = StyleSheet.create({
  headerBar: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.sm },
  backBtn: {
    width: 36,
    height: 36,
    alignItems: "center",
    justifyContent: "center",
    borderRadius: theme.radii.pill,
  },
  scroll: { paddingBottom: theme.spacing.huge, gap: theme.spacing.md },
  card: { marginHorizontal: theme.spacing.lg },
  input: {
    fontFamily: theme.fonts.sans,
    fontSize: 16,
    color: theme.colors.text,
    paddingVertical: theme.spacing.sm,
  },
  row: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: theme.spacing.lg,
    paddingVertical: theme.spacing.md,
    minHeight: 56,
    gap: theme.spacing.md,
  },
  actions: { paddingHorizontal: theme.spacing.lg, marginTop: theme.spacing.sm },
  center: { paddingVertical: theme.spacing.huge, alignItems: "center" },
});
