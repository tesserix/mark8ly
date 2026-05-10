import { useState } from "react";
import {
  ActivityIndicator,
  Alert,
  KeyboardAvoidingView,
  Platform,
  ScrollView,
  StyleSheet,
  Switch,
  TextInput,
  TouchableOpacity,
  View,
} from "react-native";
import { Stack, useRouter } from "expo-router";
import { ChevronLeft, MapPin, Plus, Trash2 } from "lucide-react-native";
import {
  type AddressInput,
  useAddresses,
  useCreateAddress,
  useDeleteAddress,
  useUpdateAddress,
} from "@/lib/hooks/use-account";
import {
  Button,
  Card,
  EmptyState,
  Hairline,
  PageHeader,
  Screen,
  Text,
} from "@/components/ui";
import { theme } from "@/lib/theme";
import type { StorefrontAddress } from "@repo/mobile-shared/api/storefront-types";

const EMPTY: AddressInput = {
  name: "",
  line1: "",
  line2: "",
  city: "",
  region: "",
  postal_code: "",
  country: "",
  is_default: false,
};

export default function AddressesScreen() {
  const router = useRouter();
  const { data, isLoading, refetch } = useAddresses();
  const create = useCreateAddress();
  const update = useUpdateAddress();
  const del = useDeleteAddress();

  const [editing, setEditing] = useState<{ id?: string; values: AddressInput } | null>(null);

  const items = data?.items ?? [];

  const handleSave = () => {
    if (!editing) return;
    const values = editing.values;
    const onError = (err: unknown) =>
      Alert.alert(
        "Couldn't save",
        err instanceof Error ? err.message : "Try again.",
      );
    if (editing.id) {
      update.mutate(
        { id: editing.id, ...values },
        { onSuccess: () => setEditing(null), onError },
      );
    } else {
      create.mutate(values, { onSuccess: () => setEditing(null), onError });
    }
  };

  const handleDelete = (id: string) =>
    Alert.alert("Delete address", "Are you sure?", [
      { text: "Cancel", style: "cancel" },
      {
        text: "Delete",
        style: "destructive",
        onPress: () => del.mutate(id),
      },
    ]);

  return (
    <Screen>
      <Stack.Screen options={{ headerShown: false }} />
      <KeyboardAvoidingView
        style={{ flex: 1 }}
        behavior={Platform.OS === "ios" ? "padding" : undefined}
      >
        <View style={styles.headerBar}>
          <TouchableOpacity
            onPress={() => (editing ? setEditing(null) : router.back())}
            hitSlop={12}
            accessibilityRole="button"
            accessibilityLabel={editing ? "Cancel" : "Back"}
            style={styles.backBtn}
          >
            <ChevronLeft size={22} color={theme.colors.text} strokeWidth={1.75} />
          </TouchableOpacity>
        </View>

        {editing ? (
          <ScrollView
            contentContainerStyle={styles.scroll}
            keyboardShouldPersistTaps="handled"
          >
            <PageHeader
              eyebrow="ADDRESS"
              title={editing.id ? "Edit address" : "Add address"}
            />
            <Card padding="md" style={styles.card}>
              <AddressFields
                values={editing.values}
                onChange={(patch) =>
                  setEditing({
                    ...editing,
                    values: { ...editing.values, ...patch },
                  })
                }
              />
            </Card>
            <View style={styles.actions}>
              <Button
                label={editing.id ? "Save changes" : "Save address"}
                onPress={handleSave}
                loading={create.isPending || update.isPending}
                fullWidth
              />
              {editing.id ? (
                <TouchableOpacity
                  onPress={() => handleDelete(editing.id!)}
                  hitSlop={8}
                  style={{ alignSelf: "center", marginTop: theme.spacing.md }}
                  accessibilityRole="button"
                  accessibilityLabel="Delete address"
                >
                  <Text preset="bodyEmphasis" color="danger">
                    Delete address
                  </Text>
                </TouchableOpacity>
              ) : null}
            </View>
          </ScrollView>
        ) : (
          <>
            <PageHeader
              eyebrow="ACCOUNT"
              title="Addresses"
              subtitle={items.length ? undefined : "Save your shipping details for faster checkout."}
              rightSlot={
                <TouchableOpacity
                  onPress={() => setEditing({ values: EMPTY })}
                  hitSlop={8}
                  accessibilityLabel="Add address"
                  style={styles.addBtn}
                >
                  <Plus size={18} color={theme.colors.text} strokeWidth={1.75} />
                </TouchableOpacity>
              }
            />
            {isLoading ? (
              <View style={styles.center}>
                <ActivityIndicator size="small" color={theme.colors.text} />
              </View>
            ) : items.length === 0 ? (
              <View style={styles.center}>
                <EmptyState
                  icon={<MapPin size={28} color={theme.colors.textTertiary} strokeWidth={1.5} />}
                  title="No addresses yet"
                  message="Add an address to check out faster."
                  action={
                    <Button
                      label="Add address"
                      onPress={() => setEditing({ values: EMPTY })}
                    />
                  }
                />
              </View>
            ) : (
              <ScrollView
                contentContainerStyle={styles.scroll}
                refreshControl={undefined}
                onScrollEndDrag={() => refetch()}
              >
                {items.map((address) => (
                  <Card key={address.id} padding={0} style={styles.card}>
                    <TouchableOpacity
                      activeOpacity={0.6}
                      onPress={() =>
                        setEditing({
                          id: address.id,
                          values: { ...address },
                        })
                      }
                      accessibilityRole="button"
                      accessibilityLabel={`Edit address: ${address.name}`}
                      style={styles.addressRow}
                    >
                      <View style={{ flex: 1, gap: 2 }}>
                        <View style={styles.addressTitleRow}>
                          <Text preset="bodyEmphasis" color="text">
                            {address.name}
                          </Text>
                          {address.is_default ? (
                            <View style={styles.defaultBadge}>
                              <Text preset="caption" color="inverse" style={{ fontSize: 10, fontWeight: "700" }}>
                                DEFAULT
                              </Text>
                            </View>
                          ) : null}
                        </View>
                        <Text preset="caption" color="textSecondary">
                          {address.line1}
                          {address.line2 ? `, ${address.line2}` : ""}
                        </Text>
                        <Text preset="caption" color="textSecondary">
                          {address.city}, {address.region} {address.postal_code} ·{" "}
                          {address.country}
                        </Text>
                      </View>
                      <TouchableOpacity
                        onPress={() => handleDelete(address.id)}
                        hitSlop={12}
                        accessibilityLabel={`Delete ${address.name}`}
                        style={styles.deleteBtn}
                      >
                        <Trash2 size={16} color={theme.colors.danger} strokeWidth={1.75} />
                      </TouchableOpacity>
                    </TouchableOpacity>
                  </Card>
                ))}
              </ScrollView>
            )}
          </>
        )}
      </KeyboardAvoidingView>
    </Screen>
  );
}

function AddressFields({
  values,
  onChange,
}: {
  values: AddressInput;
  onChange: (patch: Partial<AddressInput>) => void;
}) {
  return (
    <>
      <Field label="Name">
        <TextInput
          value={values.name}
          onChangeText={(name) => onChange({ name })}
          style={styles.input}
          placeholder="Jane Doe"
          placeholderTextColor={theme.colors.textTertiary}
          autoComplete="name"
        />
      </Field>
      <Hairline style={styles.divider} />
      <Field label="Address line 1">
        <TextInput
          value={values.line1}
          onChangeText={(line1) => onChange({ line1 })}
          style={styles.input}
          placeholder="123 Market St"
          placeholderTextColor={theme.colors.textTertiary}
          autoComplete="street-address"
        />
      </Field>
      <Hairline style={styles.divider} />
      <Field label="Address line 2 (optional)">
        <TextInput
          value={values.line2}
          onChangeText={(line2) => onChange({ line2 })}
          style={styles.input}
          placeholder="Apt 4B"
          placeholderTextColor={theme.colors.textTertiary}
        />
      </Field>
      <Hairline style={styles.divider} />
      <View style={styles.fieldRow}>
        <Field label="City" style={{ flex: 1 }}>
          <TextInput
            value={values.city}
            onChangeText={(city) => onChange({ city })}
            style={styles.input}
            placeholder="San Francisco"
            placeholderTextColor={theme.colors.textTertiary}
            autoComplete="postal-address-locality"
          />
        </Field>
        <Field label="State / Region" style={{ flex: 1 }}>
          <TextInput
            value={values.region}
            onChangeText={(region) => onChange({ region })}
            style={styles.input}
            placeholder="CA"
            placeholderTextColor={theme.colors.textTertiary}
            autoComplete="postal-address-region"
          />
        </Field>
      </View>
      <Hairline style={styles.divider} />
      <View style={styles.fieldRow}>
        <Field label="Postal code" style={{ flex: 1 }}>
          <TextInput
            value={values.postal_code}
            onChangeText={(postal_code) => onChange({ postal_code })}
            style={styles.input}
            placeholder="94103"
            placeholderTextColor={theme.colors.textTertiary}
            autoComplete="postal-code"
            keyboardType="number-pad"
          />
        </Field>
        <Field label="Country" style={{ flex: 1 }}>
          <TextInput
            value={values.country}
            onChangeText={(country) => onChange({ country })}
            style={styles.input}
            placeholder="US"
            placeholderTextColor={theme.colors.textTertiary}
            autoCapitalize="characters"
          />
        </Field>
      </View>
      <Hairline style={styles.divider} />
      <View style={styles.toggleRow}>
        <Text preset="bodyEmphasis" color="text">
          Default address
        </Text>
        <Switch
          value={values.is_default}
          onValueChange={(is_default) => onChange({ is_default })}
        />
      </View>
    </>
  );
}

function Field({
  label,
  children,
  style,
}: {
  label: string;
  children: React.ReactNode;
  style?: { flex?: number };
}) {
  return (
    <View style={[{ gap: theme.spacing.xs }, style]}>
      <Text preset="caption" color="textTertiary">
        {label.toUpperCase()}
      </Text>
      {children}
    </View>
  );
}

// Address type re-exported for convenience.
export type { StorefrontAddress };

const styles = StyleSheet.create({
  headerBar: { paddingHorizontal: theme.spacing.lg, paddingTop: theme.spacing.sm },
  backBtn: {
    width: 36,
    height: 36,
    alignItems: "center",
    justifyContent: "center",
    borderRadius: theme.radii.pill,
  },
  addBtn: {
    width: 36,
    height: 36,
    alignItems: "center",
    justifyContent: "center",
    borderRadius: theme.radii.pill,
    backgroundColor: theme.colors.surfaceAlt,
  },
  scroll: { paddingBottom: theme.spacing.huge, gap: theme.spacing.sm },
  card: { marginHorizontal: theme.spacing.lg },
  addressRow: {
    flexDirection: "row",
    alignItems: "center",
    padding: theme.spacing.md,
  },
  addressTitleRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: theme.spacing.sm,
  },
  defaultBadge: {
    backgroundColor: theme.colors.accent,
    paddingHorizontal: theme.spacing.xs,
    paddingVertical: 2,
    borderRadius: theme.radii.sm,
  },
  deleteBtn: { padding: theme.spacing.sm },
  input: {
    fontFamily: theme.fonts.sans,
    fontSize: 16,
    color: theme.colors.text,
    paddingVertical: theme.spacing.sm,
  },
  fieldRow: { flexDirection: "row", gap: theme.spacing.md },
  toggleRow: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingVertical: theme.spacing.sm,
  },
  divider: { marginVertical: 2 },
  actions: { paddingHorizontal: theme.spacing.lg, marginTop: theme.spacing.md },
  center: { paddingVertical: theme.spacing.huge, alignItems: "center" },
});
