import { useState, useCallback, useEffect, useMemo } from "react";
import {
  View,
  Text,
  TextInput,
  ScrollView,
  Pressable,
  StyleSheet,
  ActivityIndicator,
} from "react-native";
import { useTheme } from "@/lib/theme/theme-provider";
import { useRouter } from "expo-router";
import { useAuth } from "@repo/mobile-shared/auth/provider";
import { haptics } from "@repo/mobile-shared/haptics/feedback";
import { useCheckoutStore, type CheckoutAddress } from "@/stores/checkout-store";
import {
  useCheckCustomerEmail,
  useSavedAddresses,
} from "@/lib/hooks/use-checkout";
import { LoginModal } from "@/components/LoginModal";

const COUNTRIES = [
  "AU", "CA", "DE", "ES", "FR", "GB", "ID", "IN", "IT", "MY", "NL", "PH",
  "SG", "TH", "US",
] as const;

type CountryCode = (typeof COUNTRIES)[number];

interface FormErrors {
  email?: string;
  customerName?: string;
  name?: string;
  line1?: string;
  city?: string;
  region?: string;
  postalCode?: string;
  country?: string;
}

export default function CheckoutDetailsScreen() {
  const theme = useTheme();
  const styles = useMemo(() => createThemedStyles(theme), [theme]);
  const router = useRouter();
  const { user } = useAuth();
  const isAuthenticated = !!user;

  const checkoutStore = useCheckoutStore();
  const checkEmail = useCheckCustomerEmail();
  const { data: savedAddresses } = useSavedAddresses();

  // Contact fields
  const [email, setEmail] = useState(checkoutStore.email || user?.email || "");
  const [customerName, setCustomerName] = useState(
    checkoutStore.customerName || user?.displayName || "",
  );

  // Account recognition
  const [accountFound, setAccountFound] = useState(false);
  const [loginModalVisible, setLoginModalVisible] = useState(false);

  // Address selection
  const [selectedSavedId, setSelectedSavedId] = useState<string | null>(
    checkoutStore.selectedAddressId,
  );
  const [showNewAddress, setShowNewAddress] = useState(
    !isAuthenticated || !savedAddresses?.length,
  );

  // Address form
  const [addressName, setAddressName] = useState(
    checkoutStore.address?.line1 ? customerName : "",
  );
  const [line1, setLine1] = useState(checkoutStore.address?.line1 ?? "");
  const [line2, setLine2] = useState(checkoutStore.address?.line2 ?? "");
  const [city, setCity] = useState(checkoutStore.address?.city ?? "");
  const [region, setRegion] = useState(checkoutStore.address?.state ?? "");
  const [postalCode, setPostalCode] = useState(
    checkoutStore.address?.postalCode ?? "",
  );
  const [country, setCountry] = useState<CountryCode>(
    (checkoutStore.address?.country as CountryCode) || "US",
  );
  const [saveAddress, setSaveAddress] = useState(checkoutStore.saveAddress);
  const [errors, setErrors] = useState<FormErrors>({});

  // Show new address form if no saved addresses
  useEffect(() => {
    if (isAuthenticated && savedAddresses && savedAddresses.length > 0) {
      setShowNewAddress(false);
    } else {
      setShowNewAddress(true);
    }
  }, [isAuthenticated, savedAddresses]);

  const handleEmailBlur = useCallback(async () => {
    if (!email.trim() || isAuthenticated) return;

    try {
      const result = await checkEmail.mutateAsync(email.trim());
      if (result.ok && result.status === 200) {
        setAccountFound(true);
      } else {
        setAccountFound(false);
      }
    } catch {
      setAccountFound(false);
    }
  }, [email, isAuthenticated, checkEmail]);

  const handleLoginSuccess = useCallback(() => {
    setLoginModalVisible(false);
    setAccountFound(false);
  }, []);

  const validate = useCallback((): boolean => {
    const next: FormErrors = {};

    if (!email.trim()) next.email = "Email is required";
    else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim()))
      next.email = "Enter a valid email";

    if (!customerName.trim()) next.customerName = "Name is required";

    if (showNewAddress && !selectedSavedId) {
      if (!line1.trim()) next.line1 = "Address is required";
      if (!city.trim()) next.city = "City is required";
      if (!region.trim()) next.region = "Region is required";
      if (!postalCode.trim()) next.postalCode = "Postal code is required";
      if (!country) next.country = "Country is required";
    }

    setErrors(next);
    return Object.keys(next).length === 0;
  }, [
    email,
    customerName,
    showNewAddress,
    selectedSavedId,
    line1,
    city,
    region,
    postalCode,
    country,
  ]);

  const handleContinue = useCallback(async () => {
    if (!validate()) return;

    await haptics.checkoutStep();

    checkoutStore.setContact(email.trim(), customerName.trim());
    checkoutStore.setStep(2);

    if (selectedSavedId && !showNewAddress) {
      checkoutStore.setSelectedAddressId(selectedSavedId);
    } else {
      const address: CheckoutAddress = {
        line1: line1.trim(),
        line2: line2.trim(),
        city: city.trim(),
        state: region.trim(),
        postalCode: postalCode.trim(),
        country,
      };
      checkoutStore.setAddress(address);
    }

    checkoutStore.setSaveAddress(saveAddress);
    router.push("/checkout/shipping");
  }, [
    validate,
    checkoutStore,
    email,
    customerName,
    selectedSavedId,
    showNewAddress,
    line1,
    line2,
    city,
    region,
    postalCode,
    country,
    saveAddress,
    router,
  ]);

  const handleSelectSaved = useCallback(
    (id: string) => {
      setSelectedSavedId(id);
      setShowNewAddress(false);
    },
    [],
  );

  const handleAddNew = useCallback(() => {
    setSelectedSavedId(null);
    setShowNewAddress(true);
  }, []);

  return (
    <View style={styles.container}>
      <ScrollView
        contentContainerStyle={styles.scrollContent}
        showsVerticalScrollIndicator={false}
        keyboardShouldPersistTaps="handled"
      >
        {/* Contact section */}
        <View style={styles.section}>
          <Text style={styles.sectionTitle}>Contact</Text>

          <View style={styles.field}>
            <Text style={styles.label}>Email</Text>
            <TextInput
              style={[styles.input, errors.email && styles.inputError]}
              value={email}
              onChangeText={setEmail}
              onBlur={handleEmailBlur}
              placeholder="you@example.com"
              placeholderTextColor={theme.textSecondary}
              keyboardType="email-address"
              autoCapitalize="none"
              autoComplete="email"
              textContentType="emailAddress"
              accessibilityLabel="Email address"
            />
            {errors.email && (
              <Text style={styles.errorText}>{errors.email}</Text>
            )}
            {accountFound && !isAuthenticated && (
              <View style={styles.nudgeBanner}>
                <Text style={styles.nudgeText}>
                  We found an account with this email.{" "}
                </Text>
                <Pressable onPress={() => setLoginModalVisible(true)} accessibilityRole="button">
                  <Text style={styles.nudgeLink}>Sign in</Text>
                </Pressable>
                <Text style={styles.nudgeText}>
                  {" "}for saved addresses.
                </Text>
              </View>
            )}
          </View>

          <View style={styles.field}>
            <Text style={styles.label}>Full name</Text>
            <TextInput
              style={[styles.input, errors.customerName && styles.inputError]}
              value={customerName}
              onChangeText={setCustomerName}
              placeholder="Jane Doe"
              placeholderTextColor={theme.textSecondary}
              autoComplete="name"
              textContentType="name"
              accessibilityLabel="Full name"
            />
            {errors.customerName && (
              <Text style={styles.errorText}>{errors.customerName}</Text>
            )}
          </View>
        </View>

        {/* Divider */}
        <View style={styles.divider} />

        {/* Address section */}
        <View style={styles.section}>
          <Text style={styles.sectionTitle}>Shipping address</Text>

          {/* Saved addresses */}
          {isAuthenticated &&
            savedAddresses &&
            savedAddresses.length > 0 && (
              <View style={styles.savedList}>
                {savedAddresses.map((addr) => (
                  <Pressable
                    key={addr.id}
                    style={[
                      styles.savedCard,
                      selectedSavedId === addr.id &&
                        !showNewAddress &&
                        styles.savedCardSelected,
                    ]}
                    onPress={() => handleSelectSaved(addr.id)}
                    accessibilityRole="radio"
                    accessibilityState={{
                      selected:
                        selectedSavedId === addr.id && !showNewAddress,
                    }}
                  >
                    <View style={styles.radioOuter}>
                      {selectedSavedId === addr.id && !showNewAddress && (
                        <View style={styles.radioInner} />
                      )}
                    </View>
                    <View style={styles.savedText}>
                      <Text style={styles.savedName}>{addr.name}</Text>
                      <Text style={styles.savedAddr}>
                        {addr.line1}
                        {addr.line2 ? `, ${addr.line2}` : ""}
                      </Text>
                      <Text style={styles.savedAddr}>
                        {addr.city}, {addr.region} {addr.postal_code}
                      </Text>
                      <Text style={styles.savedAddr}>{addr.country}</Text>
                    </View>
                    {addr.is_default && (
                      <View style={styles.defaultBadge}>
                        <Text style={styles.defaultBadgeText}>Default</Text>
                      </View>
                    )}
                  </Pressable>
                ))}

                <Pressable
                  style={[
                    styles.savedCard,
                    showNewAddress && styles.savedCardSelected,
                  ]}
                  onPress={handleAddNew}
                  accessibilityRole="radio"
                  accessibilityState={{ selected: showNewAddress }}
                >
                  <View style={styles.radioOuter}>
                    {showNewAddress && <View style={styles.radioInner} />}
                  </View>
                  <Text style={styles.addNewText}>Add new address</Text>
                </Pressable>
              </View>
            )}

          {/* New address form */}
          {showNewAddress && (
            <View style={styles.addressForm}>
              <View style={styles.field}>
                <Text style={styles.label}>Name</Text>
                <TextInput
                  style={styles.input}
                  value={addressName}
                  onChangeText={setAddressName}
                  placeholder="Recipient name"
                  placeholderTextColor={theme.textSecondary}
                  accessibilityLabel="Recipient name"
                />
              </View>

              <View style={styles.field}>
                <Text style={styles.label}>Address line 1</Text>
                <TextInput
                  style={[styles.input, errors.line1 && styles.inputError]}
                  value={line1}
                  onChangeText={setLine1}
                  placeholder="Street address"
                  placeholderTextColor={theme.textSecondary}
                  autoComplete="street-address"
                  textContentType="streetAddressLine1"
                  accessibilityLabel="Address line 1"
                />
                {errors.line1 && (
                  <Text style={styles.errorText}>{errors.line1}</Text>
                )}
              </View>

              <View style={styles.field}>
                <Text style={styles.label}>Address line 2 (optional)</Text>
                <TextInput
                  style={styles.input}
                  value={line2}
                  onChangeText={setLine2}
                  placeholder="Apartment, suite, etc."
                  placeholderTextColor={theme.textSecondary}
                  textContentType="streetAddressLine2"
                  accessibilityLabel="Address line 2"
                />
              </View>

              <View style={styles.row}>
                <View style={[styles.field, styles.fieldHalf]}>
                  <Text style={styles.label}>City</Text>
                  <TextInput
                    style={[styles.input, errors.city && styles.inputError]}
                    value={city}
                    onChangeText={setCity}
                    placeholder="City"
                    placeholderTextColor={theme.textSecondary}
                    textContentType="addressCity"
                    accessibilityLabel="City"
                  />
                  {errors.city && (
                    <Text style={styles.errorText}>{errors.city}</Text>
                  )}
                </View>

                <View style={[styles.field, styles.fieldHalf]}>
                  <Text style={styles.label}>Region / State</Text>
                  <TextInput
                    style={[styles.input, errors.region && styles.inputError]}
                    value={region}
                    onChangeText={setRegion}
                    placeholder="Region"
                    placeholderTextColor={theme.textSecondary}
                    textContentType="addressState"
                    accessibilityLabel="Region or state"
                  />
                  {errors.region && (
                    <Text style={styles.errorText}>{errors.region}</Text>
                  )}
                </View>
              </View>

              <View style={styles.row}>
                <View style={[styles.field, styles.fieldHalf]}>
                  <Text style={styles.label}>Postal code</Text>
                  <TextInput
                    style={[
                      styles.input,
                      errors.postalCode && styles.inputError,
                    ]}
                    value={postalCode}
                    onChangeText={setPostalCode}
                    placeholder="Postal code"
                    placeholderTextColor={theme.textSecondary}
                    textContentType="postalCode"
                    accessibilityLabel="Postal code"
                  />
                  {errors.postalCode && (
                    <Text style={styles.errorText}>{errors.postalCode}</Text>
                  )}
                </View>

                <View style={[styles.field, styles.fieldHalf]}>
                  <Text style={styles.label}>Country</Text>
                  <View style={styles.countryPicker}>
                    <ScrollView
                      horizontal
                      showsHorizontalScrollIndicator={false}
                      contentContainerStyle={styles.countryChips}
                    >
                      {COUNTRIES.map((code) => (
                        <Pressable
                          key={code}
                          style={[
                            styles.countryChip,
                            country === code && styles.countryChipActive,
                          ]}
                          onPress={() => setCountry(code)}
                          accessibilityRole="radio"
                          accessibilityState={{ selected: country === code }}
                          accessibilityLabel={code}
                        >
                          <Text
                            style={[
                              styles.countryChipText,
                              country === code &&
                                styles.countryChipTextActive,
                            ]}
                          >
                            {code}
                          </Text>
                        </Pressable>
                      ))}
                    </ScrollView>
                  </View>
                </View>
              </View>

              {isAuthenticated && (
                <Pressable
                  style={styles.checkboxRow}
                  onPress={() => setSaveAddress((prev) => !prev)}
                  accessibilityRole="checkbox"
                  accessibilityState={{ checked: saveAddress }}
                >
                  <View
                    style={[
                      styles.checkbox,
                      saveAddress && styles.checkboxChecked,
                    ]}
                  >
                    {saveAddress && (
                      <Text style={styles.checkmark}>✓</Text>
                    )}
                  </View>
                  <Text style={styles.checkboxLabel}>
                    Save this address for next time
                  </Text>
                </Pressable>
              )}
            </View>
          )}
        </View>
      </ScrollView>

      {/* Sticky continue */}
      <View style={styles.stickyBar}>
        <Pressable
          style={styles.continueButton}
          onPress={handleContinue}
          accessibilityRole="button"
          accessibilityLabel="Continue to shipping"
        >
          <Text style={styles.continueButtonText}>Continue</Text>
        </Pressable>
      </View>

      <LoginModal
        visible={loginModalVisible}
        initialEmail={email}
        onDismiss={() => setLoginModalVisible(false)}
        onSuccess={handleLoginSuccess}
      />
    </View>
  );
}

function createThemedStyles(theme: { primary: string; accent: string; background: string; elevated: string; text: string; textSecondary: string; border: string; fontFamily: string }) {
  return StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: theme.background,
  },
  scrollContent: {
    paddingBottom: 100,
  },
  section: {
    padding: 16,
    gap: 16,
  },
  sectionTitle: {
    fontSize: 18,
    fontWeight: "700",
    color: theme.text,
    fontFamily: "SourceSerif4",
  },
  divider: {
    height: StyleSheet.hairlineWidth,
    backgroundColor: theme.border,
  },
  field: {
    gap: 6,
  },
  fieldHalf: {
    flex: 1,
  },
  label: {
    fontSize: 13,
    fontWeight: "600",
    color: theme.text,
  },
  input: {
    borderWidth: 1,
    borderColor: theme.border,
    borderRadius: 6,
    paddingHorizontal: 12,
    paddingVertical: 12,
    fontSize: 15,
    color: theme.text,
    backgroundColor: theme.elevated,
  },
  inputError: {
    borderColor: "#8B2500",
  },
  errorText: {
    fontSize: 12,
    color: "#8B2500",
  },
  nudgeBanner: {
    flexDirection: "row",
    flexWrap: "wrap",
    alignItems: "center",
    backgroundColor: "#EDF4ED",
    paddingHorizontal: 10,
    paddingVertical: 8,
    borderRadius: 6,
  },
  nudgeText: {
    fontSize: 13,
    color: theme.text,
  },
  nudgeLink: {
    fontSize: 13,
    color: theme.accent,
    fontWeight: "700",
    textDecorationLine: "underline",
  },
  row: {
    flexDirection: "row",
    gap: 12,
  },
  savedList: {
    gap: 8,
  },
  savedCard: {
    flexDirection: "row",
    alignItems: "flex-start",
    backgroundColor: theme.elevated,
    padding: 14,
    borderRadius: 6,
    borderWidth: 1,
    borderColor: theme.border,
    gap: 10,
  },
  savedCardSelected: {
    borderColor: theme.primary,
  },
  radioOuter: {
    width: 20,
    height: 20,
    borderRadius: 10,
    borderWidth: 2,
    borderColor: theme.border,
    alignItems: "center",
    justifyContent: "center",
    marginTop: 2,
  },
  radioInner: {
    width: 10,
    height: 10,
    borderRadius: 5,
    backgroundColor: theme.primary,
  },
  savedText: {
    flex: 1,
    gap: 2,
  },
  savedName: {
    fontSize: 14,
    fontWeight: "600",
    color: theme.text,
  },
  savedAddr: {
    fontSize: 13,
    color: theme.textSecondary,
  },
  defaultBadge: {
    backgroundColor: "#EDF4ED",
    paddingHorizontal: 8,
    paddingVertical: 2,
    borderRadius: 4,
  },
  defaultBadgeText: {
    fontSize: 11,
    fontWeight: "600",
    color: theme.accent,
  },
  addNewText: {
    fontSize: 14,
    fontWeight: "600",
    color: theme.accent,
  },
  addressForm: {
    gap: 14,
  },
  countryPicker: {
    height: 38,
  },
  countryChips: {
    gap: 6,
    alignItems: "center",
  },
  countryChip: {
    paddingHorizontal: 10,
    paddingVertical: 8,
    borderRadius: 6,
    backgroundColor: theme.elevated,
    borderWidth: 1,
    borderColor: theme.border,
  },
  countryChipActive: {
    backgroundColor: theme.primary,
    borderColor: theme.primary,
  },
  countryChipText: {
    fontSize: 12,
    fontWeight: "600",
    color: theme.text,
  },
  countryChipTextActive: {
    color: theme.elevated,
  },
  checkboxRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: 10,
    marginTop: 4,
  },
  checkbox: {
    width: 22,
    height: 22,
    borderRadius: 4,
    borderWidth: 1.5,
    borderColor: theme.border,
    alignItems: "center",
    justifyContent: "center",
    backgroundColor: theme.elevated,
  },
  checkboxChecked: {
    backgroundColor: theme.primary,
    borderColor: theme.primary,
  },
  checkmark: {
    color: theme.elevated,
    fontSize: 13,
    fontWeight: "700",
  },
  checkboxLabel: {
    fontSize: 14,
    color: theme.text,
  },
  stickyBar: {
    position: "absolute",
    bottom: 0,
    left: 0,
    right: 0,
    backgroundColor: theme.elevated,
    paddingHorizontal: 16,
    paddingTop: 12,
    paddingBottom: 32,
    borderTopWidth: StyleSheet.hairlineWidth,
    borderTopColor: theme.border,
  },
  continueButton: {
    backgroundColor: theme.primary,
    paddingVertical: 16,
    borderRadius: 6,
    alignItems: "center",
  },
  continueButtonText: {
    color: theme.elevated,
    fontSize: 16,
    fontWeight: "600",
  },
});
}
