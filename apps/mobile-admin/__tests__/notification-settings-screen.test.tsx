// Task 9: the master push toggle and the five PREFERENCE_TYPES rows move
// from a hand-rolled `switchRow` (no Card, no consistent density) onto
// GroupedList/GroupedRow. The optimistic local mirror at `AlertTypesSection`
// (flips a switch instantly, reverts on mutation error) is explicitly
// untouched by this migration — every assertion on it here must still pass.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));
jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});

const mockPushPreference: Record<string, unknown> = {
  enabled: true,
  permission: "granted",
  loading: false,
  busy: false,
  setPushEnabled: jest.fn().mockResolvedValue({ ok: true }),
};
jest.mock("@/lib/hooks/use-push-preference", () => ({
  usePushPreference: () => mockPushPreference,
}));

const mockPreferences = {
  new_order: true,
  low_stock: true,
  return_requested: true,
  payment_received: true,
  review_submitted: true,
};
const mockUpdateMutation = { mutate: jest.fn(), isPending: false };
let mockPreferencesResult: {
  data: typeof mockPreferences | undefined;
  isLoading: boolean;
  isError: boolean;
  refetch: jest.Mock;
} = { data: mockPreferences, isLoading: false, isError: false, refetch: jest.fn() };

// NOT `jest.requireActual` here: the real module imports `useApiClient`,
// which pulls in the auth provider, which imports `expo/virtual/env` — a
// module chain this suite has no reason to boot (same landmine as the
// product-status/customer-actions mutation stubs in
// filter-chips-rollout.test.tsx). `PREFERENCE_TYPES` is re-declared as a
// literal instead, mirroring the real module's own list exactly.
jest.mock("@/lib/hooks/use-notification-preferences", () => ({
  PREFERENCE_TYPES: [
    "new_order",
    "low_stock",
    "return_requested",
    "payment_received",
    "review_submitted",
  ],
  useNotificationPreferences: () => mockPreferencesResult,
  useUpdateNotificationPreferences: () => mockUpdateMutation,
}));

import { StyleSheet } from "react-native";
import { fireEvent, render, waitFor } from "@testing-library/react-native";
import NotificationSettingsScreen from "../app/(tabs)/more/settings/notification-settings";
import { theme } from "@/lib/theme";

describe("NotificationSettingsScreen", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    Object.assign(mockPushPreference, {
      enabled: true,
      permission: "granted",
      loading: false,
      busy: false,
      setPushEnabled: jest.fn().mockResolvedValue({ ok: true }),
    });
    mockPreferencesResult = {
      data: { ...mockPreferences },
      isLoading: false,
      isError: false,
      refetch: jest.fn(),
    };
    mockUpdateMutation.mutate = jest.fn();
    mockUpdateMutation.isPending = false;
  });

  it("renders the master push toggle with its device-level description", () => {
    const { getByText, getByLabelText } = render(<NotificationSettingsScreen />);
    expect(getByText("Push notifications")).toBeTruthy();
    expect(getByText("Alerts on this device when something needs you.")).toBeTruthy();
    expect(getByLabelText("Toggle push notifications").props.value).toBe(true);
  });

  it("toggles the master switch through the same setPushEnabled call", () => {
    const { getByLabelText } = render(<NotificationSettingsScreen />);
    fireEvent(getByLabelText("Toggle push notifications"), "valueChange", false);
    expect(mockPushPreference.setPushEnabled).toHaveBeenCalledWith(false);
  });

  it("renders all five alert-type rows with their label + hint copy", () => {
    const { getByText } = render(<NotificationSettingsScreen />);
    expect(getByText("New orders")).toBeTruthy();
    expect(getByText("When a customer places an order")).toBeTruthy();
    expect(getByText("Low stock")).toBeTruthy();
    expect(getByText("Return requests")).toBeTruthy();
    expect(getByText("Payments")).toBeTruthy();
    expect(getByText("New reviews")).toBeTruthy();
  });

  // The optimistic local mirror, unchanged by this migration: the switch
  // must flip THE INSTANT it's tapped, before the mutation resolves.
  it("flips a preference switch immediately (optimistic), before the mutation resolves", async () => {
    const { getByLabelText } = render(<NotificationSettingsScreen />);
    const sw = getByLabelText("Toggle New orders notifications");
    expect(sw.props.value).toBe(true);
    fireEvent(sw, "valueChange", false);
    await waitFor(() => expect(getByLabelText("Toggle New orders notifications").props.value).toBe(false));
  });

  it("sends the FULL preference set on toggle, not just the changed key", () => {
    const { getByLabelText } = render(<NotificationSettingsScreen />);
    fireEvent(getByLabelText("Toggle Low stock notifications"), "valueChange", false);
    expect(mockUpdateMutation.mutate).toHaveBeenCalledWith(
      expect.objectContaining({ ...mockPreferences, low_stock: false }),
      expect.anything(),
    );
  });

  it("renders the intro copy as a footer caption under the alert-types card", () => {
    const { getByText } = render(<NotificationSettingsScreen />);
    expect(
      getByText(/Choose what your store notifies you about/),
    ).toBeTruthy();
  });

  it("shows the blocked-permission warning and an Open device settings link", () => {
    mockPushPreference.permission = "denied";
    const { getByText } = render(<NotificationSettingsScreen />);
    expect(getByText(/Notifications are blocked in your device settings/)).toBeTruthy();
    expect(getByText("Open device settings").props.accessibilityRole).toBe("link");
  });

  it("keeps the screen gutter at theme.spacing.xl", () => {
    const { UNSAFE_root } = render(<NotificationSettingsScreen />);
    const { ScrollView } = require("react-native");
    const scroll = UNSAFE_root.findByType(ScrollView);
    const style = StyleSheet.flatten(scroll.props.contentContainerStyle);
    expect(style.paddingHorizontal).toBe(theme.spacing.xl);
  });

  it("gives an alert-type row the app-wide 64pt minHeight, never a fixed height", () => {
    const { getByLabelText } = render(<NotificationSettingsScreen />);
    // GroupedRow's non-interactive View branch carries its own
    // accessibilityLabel (label + hint) distinct from the Switch nested
    // inside it.
    const row = getByLabelText("New orders, When a customer places an order");
    const style = StyleSheet.flatten(row.props.style);
    expect(style.minHeight).toBe(theme.row.minHeightSingle);
    expect(style.height).toBeUndefined();
    // No `onPress` — the row itself isn't a button, only the Switch inside it is.
    expect(row.props.accessibilityRole).not.toBe("button");
  });
});
