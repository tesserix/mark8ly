import { useState, useCallback } from "react";
import {
  View,
  Text,
  TextInput,
  StyleSheet,
  FlatList,
  Pressable,
  ActivityIndicator,
  Modal,
  ScrollView,
  Alert,
  KeyboardAvoidingView,
  Platform,
} from "react-native";
import { MapPin, Plus, Pencil, Trash2 } from "lucide-react-native";
import {
  useAddresses,
  useCreateAddress,
  useUpdateAddress,
  useDeleteAddress,
} from "@/lib/hooks/use-addresses";
import type { Address, AddressInput } from "@/lib/storefront-api/addresses";

const EMPTY_FORM: AddressInput = {
  first_name: "",
  last_name: "",
  line1: "",
  line2: "",
  city: "",
  state: "",
  postal_code: "",
  country: "",
  phone: "",
  is_default: false,
};

export default function AddressesScreen() {
  const { data: addresses, isLoading } = useAddresses();
  const [modalVisible, setModalVisible] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [form, setForm] = useState<AddressInput>(EMPTY_FORM);

  const createMutation = useCreateAddress();
  const updateMutation = useUpdateAddress();
  const deleteMutation = useDeleteAddress();

  const openCreate = useCallback(() => {
    setEditingId(null);
    setForm(EMPTY_FORM);
    setModalVisible(true);
  }, []);

  const openEdit = useCallback((address: Address) => {
    setEditingId(address.id);
    setForm({
      first_name: address.first_name,
      last_name: address.last_name,
      line1: address.line1,
      line2: address.line2 ?? "",
      city: address.city,
      state: address.state,
      postal_code: address.postal_code,
      country: address.country,
      phone: address.phone ?? "",
      is_default: address.is_default,
    });
    setModalVisible(true);
  }, []);

  const handleSave = async () => {
    if (!form.first_name.trim() || !form.line1.trim() || !form.city.trim() || !form.postal_code.trim() || !form.country.trim()) {
      Alert.alert("Missing fields", "Please fill in all required fields.");
      return;
    }

    if (editingId) {
      await updateMutation.mutateAsync({ id: editingId, body: form });
    } else {
      await createMutation.mutateAsync(form);
    }
    setModalVisible(false);
  };

  const handleDelete = (id: string) => {
    Alert.alert("Delete address", "Are you sure you want to remove this address?", [
      { text: "Cancel", style: "cancel" },
      {
        text: "Delete",
        style: "destructive",
        onPress: () => deleteMutation.mutate(id),
      },
    ]);
  };

  const handleSetDefault = (id: string) => {
    updateMutation.mutate({ id, body: { is_default: true } });
  };

  const updateField = useCallback(
    <K extends keyof AddressInput>(key: K, value: AddressInput[K]) => {
      setForm((prev) => ({ ...prev, [key]: value }));
    },
    [],
  );

  if (isLoading) {
    return (
      <View style={styles.centered}>
        <ActivityIndicator size="large" color="#0E0E0C" />
      </View>
    );
  }

  if (!addresses || addresses.length === 0) {
    return (
      <View style={styles.centered}>
        <MapPin size={48} color="#CCCCCC" />
        <Text style={styles.emptyTitle}>No saved addresses</Text>
        <Text style={styles.emptySubtitle}>
          Add an address for faster checkout.
        </Text>
        <Pressable
          style={styles.addButton}
          onPress={openCreate}
          accessibilityRole="button"
        >
          <Plus size={18} color="#FFFFFF" />
          <Text style={styles.addButtonText}>Add address</Text>
        </Pressable>

        <AddressFormModal
          visible={modalVisible}
          form={form}
          isEditing={editingId !== null}
          saving={createMutation.isPending || updateMutation.isPending}
          onUpdateField={updateField}
          onSave={handleSave}
          onDismiss={() => setModalVisible(false)}
        />
      </View>
    );
  }

  const renderItem = ({ item }: { item: Address }) => (
    <View style={styles.card}>
      <View style={styles.cardHeader}>
        <Text style={styles.cardName}>
          {item.first_name} {item.last_name}
        </Text>
        {item.is_default && (
          <View style={styles.defaultBadge}>
            <Text style={styles.defaultBadgeText}>Default</Text>
          </View>
        )}
      </View>
      <Text style={styles.cardLine}>{item.line1}</Text>
      {item.line2 ? <Text style={styles.cardLine}>{item.line2}</Text> : null}
      <Text style={styles.cardLine}>
        {item.city}, {item.state} {item.postal_code}
      </Text>
      <Text style={styles.cardLine}>{item.country}</Text>
      {item.phone ? <Text style={styles.cardPhone}>{item.phone}</Text> : null}

      <View style={styles.cardActions}>
        {!item.is_default && (
          <Pressable
            onPress={() => handleSetDefault(item.id)}
            style={styles.actionLink}
            accessibilityRole="button"
            accessibilityLabel="Set as default"
          >
            <Text style={styles.actionLinkText}>Set as default</Text>
          </Pressable>
        )}
        <View style={styles.actionIcons}>
          <Pressable
            onPress={() => openEdit(item)}
            hitSlop={8}
            accessibilityRole="button"
            accessibilityLabel="Edit address"
          >
            <Pencil size={18} color="#666666" />
          </Pressable>
          <Pressable
            onPress={() => handleDelete(item.id)}
            hitSlop={8}
            accessibilityRole="button"
            accessibilityLabel="Delete address"
          >
            <Trash2 size={18} color="#8B2020" />
          </Pressable>
        </View>
      </View>
    </View>
  );

  return (
    <View style={styles.container}>
      <FlatList
        contentContainerStyle={styles.listContent}
        data={addresses}
        keyExtractor={(item) => item.id}
        renderItem={renderItem}
      />
      <Pressable
        style={styles.fab}
        onPress={openCreate}
        accessibilityRole="button"
        accessibilityLabel="Add address"
      >
        <Plus size={24} color="#FFFFFF" />
      </Pressable>

      <AddressFormModal
        visible={modalVisible}
        form={form}
        isEditing={editingId !== null}
        saving={createMutation.isPending || updateMutation.isPending}
        onUpdateField={updateField}
        onSave={handleSave}
        onDismiss={() => setModalVisible(false)}
      />
    </View>
  );
}

interface AddressFormModalProps {
  visible: boolean;
  form: AddressInput;
  isEditing: boolean;
  saving: boolean;
  onUpdateField: <K extends keyof AddressInput>(key: K, value: AddressInput[K]) => void;
  onSave: () => void;
  onDismiss: () => void;
}

function AddressFormModal({
  visible,
  form,
  isEditing,
  saving,
  onUpdateField,
  onSave,
  onDismiss,
}: AddressFormModalProps) {
  return (
    <Modal visible={visible} transparent animationType="slide" onRequestClose={onDismiss}>
      <KeyboardAvoidingView
        style={styles.modalOverlay}
        behavior={Platform.OS === "ios" ? "padding" : "height"}
      >
        <View style={styles.modalCard}>
          <ScrollView showsVerticalScrollIndicator={false}>
            <Text style={styles.modalTitle}>
              {isEditing ? "Edit address" : "New address"}
            </Text>

            <View style={styles.modalRow}>
              <FormField
                label="First name"
                value={form.first_name}
                onChangeText={(v) => onUpdateField("first_name", v)}
                style={styles.halfField}
              />
              <FormField
                label="Last name"
                value={form.last_name}
                onChangeText={(v) => onUpdateField("last_name", v)}
                style={styles.halfField}
              />
            </View>

            <FormField
              label="Address line 1"
              value={form.line1}
              onChangeText={(v) => onUpdateField("line1", v)}
            />
            <FormField
              label="Address line 2"
              value={form.line2 ?? ""}
              onChangeText={(v) => onUpdateField("line2", v)}
            />

            <View style={styles.modalRow}>
              <FormField
                label="City"
                value={form.city}
                onChangeText={(v) => onUpdateField("city", v)}
                style={styles.halfField}
              />
              <FormField
                label="State"
                value={form.state}
                onChangeText={(v) => onUpdateField("state", v)}
                style={styles.halfField}
              />
            </View>

            <View style={styles.modalRow}>
              <FormField
                label="Postal code"
                value={form.postal_code}
                onChangeText={(v) => onUpdateField("postal_code", v)}
                style={styles.halfField}
              />
              <FormField
                label="Country"
                value={form.country}
                onChangeText={(v) => onUpdateField("country", v)}
                style={styles.halfField}
              />
            </View>

            <FormField
              label="Phone"
              value={form.phone ?? ""}
              onChangeText={(v) => onUpdateField("phone", v)}
              keyboardType="phone-pad"
            />

            <Pressable
              style={[styles.saveButton, saving && styles.saveButtonDisabled]}
              onPress={onSave}
              disabled={saving}
              accessibilityRole="button"
            >
              {saving ? (
                <ActivityIndicator size="small" color="#FFFFFF" />
              ) : (
                <Text style={styles.saveButtonText}>
                  {isEditing ? "Update" : "Save address"}
                </Text>
              )}
            </Pressable>

            <Pressable
              style={styles.cancelButton}
              onPress={onDismiss}
              disabled={saving}
              accessibilityRole="button"
            >
              <Text style={styles.cancelText}>Cancel</Text>
            </Pressable>
          </ScrollView>
        </View>
      </KeyboardAvoidingView>
    </Modal>
  );
}

function FormField({
  label,
  value,
  onChangeText,
  style,
  keyboardType,
}: {
  label: string;
  value: string;
  onChangeText: (text: string) => void;
  style?: object;
  keyboardType?: "default" | "phone-pad";
}) {
  return (
    <View style={[styles.formField, style]}>
      <Text style={styles.formLabel}>{label}</Text>
      <TextInput
        style={styles.formInput}
        value={value}
        onChangeText={onChangeText}
        placeholderTextColor="#999999"
        keyboardType={keyboardType ?? "default"}
        accessibilityLabel={label}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#F7F6F2",
  },
  listContent: {
    padding: 16,
    gap: 12,
    paddingBottom: 80,
  },
  centered: {
    flex: 1,
    backgroundColor: "#F7F6F2",
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: 32,
    gap: 10,
  },
  emptyTitle: {
    fontSize: 18,
    fontWeight: "700",
    color: "#0E0E0C",
    marginTop: 12,
  },
  emptySubtitle: {
    fontSize: 14,
    color: "#666666",
    textAlign: "center",
  },
  addButton: {
    flexDirection: "row",
    backgroundColor: "#0E0E0C",
    height: 44,
    borderRadius: 6,
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: 20,
    gap: 8,
    marginTop: 8,
  },
  addButtonText: {
    color: "#FFFFFF",
    fontSize: 15,
    fontWeight: "600",
  },
  card: {
    backgroundColor: "#FFFFFF",
    borderRadius: 6,
    padding: 16,
    gap: 2,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: "#E5E4DF",
  },
  cardHeader: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    marginBottom: 6,
  },
  cardName: {
    fontSize: 15,
    fontWeight: "700",
    color: "#0E0E0C",
  },
  defaultBadge: {
    backgroundColor: "#2D4A2B15",
    paddingHorizontal: 8,
    paddingVertical: 3,
    borderRadius: 4,
  },
  defaultBadgeText: {
    fontSize: 11,
    fontWeight: "600",
    color: "#2D4A2B",
  },
  cardLine: {
    fontSize: 13,
    color: "#666666",
    lineHeight: 18,
  },
  cardPhone: {
    fontSize: 13,
    color: "#666666",
    marginTop: 4,
  },
  cardActions: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    marginTop: 12,
    paddingTop: 10,
    borderTopWidth: StyleSheet.hairlineWidth,
    borderTopColor: "#E5E4DF",
  },
  actionLink: {},
  actionLinkText: {
    fontSize: 13,
    fontWeight: "500",
    color: "#2D4A2B",
  },
  actionIcons: {
    flexDirection: "row",
    gap: 16,
  },
  fab: {
    position: "absolute",
    bottom: 24,
    right: 20,
    width: 52,
    height: 52,
    borderRadius: 26,
    backgroundColor: "#0E0E0C",
    alignItems: "center",
    justifyContent: "center",
    shadowColor: "#000",
    shadowOffset: { width: 0, height: 2 },
    shadowOpacity: 0.15,
    shadowRadius: 6,
    elevation: 4,
  },
  modalOverlay: {
    flex: 1,
    backgroundColor: "rgba(0,0,0,0.4)",
    justifyContent: "flex-end",
  },
  modalCard: {
    backgroundColor: "#FFFFFF",
    borderTopLeftRadius: 16,
    borderTopRightRadius: 16,
    padding: 24,
    paddingBottom: 40,
    maxHeight: "85%",
  },
  modalTitle: {
    fontSize: 18,
    fontWeight: "700",
    color: "#0E0E0C",
    marginBottom: 16,
  },
  modalRow: {
    flexDirection: "row",
    gap: 12,
  },
  halfField: {
    flex: 1,
  },
  formField: {
    marginBottom: 12,
    gap: 4,
  },
  formLabel: {
    fontSize: 13,
    fontWeight: "600",
    color: "#0E0E0C",
  },
  formInput: {
    borderWidth: 1,
    borderColor: "#E5E4DF",
    borderRadius: 6,
    paddingHorizontal: 12,
    paddingVertical: 10,
    fontSize: 15,
    color: "#0E0E0C",
    backgroundColor: "#FFFFFF",
  },
  saveButton: {
    backgroundColor: "#0E0E0C",
    height: 48,
    borderRadius: 6,
    alignItems: "center",
    justifyContent: "center",
    marginTop: 8,
  },
  saveButtonDisabled: {
    opacity: 0.6,
  },
  saveButtonText: {
    color: "#FFFFFF",
    fontSize: 15,
    fontWeight: "600",
  },
  cancelButton: {
    alignItems: "center",
    paddingVertical: 12,
  },
  cancelText: {
    fontSize: 14,
    color: "#666666",
  },
});
