// Task 9: more/index.tsx's local `Row` and hand-built section loop are
// promoted into GroupedList/GroupedRow. This screen is the primitive's
// SOURCE — the construction is "more/index.tsx's, promoted, not invented" —
// so every assertion here is a behaviour-preservation guard, not a new
// feature: each one already held true against the pre-migration hand-rolled
// version and must still hold true after the extraction.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));
jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});

const mockPush = jest.fn();
jest.mock("expo-router", () => ({ useRouter: () => ({ push: mockPush }) }));

// `react-native`'s index.js does
// `require('./Libraries/Linking/Linking').default` (same shape as the Alert
// mock in account-delete.test.tsx) — a plain `{ openURL: fn }` export leaves
// `Linking` undefined.
jest.mock("react-native/Libraries/Linking/Linking", () => ({
  default: { openURL: jest.fn() },
}));

type NotificationsResult = {
  data: { notifications: { is_read: boolean }[] } | undefined;
};
let mockNotifications: NotificationsResult = { data: { notifications: [] } };
jest.mock("../lib/hooks/use-notifications", () => ({
  useNotifications: () => mockNotifications,
}));

import { StyleSheet } from "react-native";
import { fireEvent, render } from "@testing-library/react-native";
import { Linking } from "react-native";
import MoreScreen from "../app/(tabs)/more/index";
import { theme } from "@/lib/theme";

function setUnread(count: number) {
  mockNotifications = {
    data: {
      notifications: Array.from({ length: count }, () => ({ is_read: false })),
    },
  };
}

describe("MoreScreen", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    setUnread(0);
  });

  it("renders one row per nav item across every section", () => {
    const { getByText } = render(<MoreScreen />);
    for (const label of [
      "Branding",
      "Team",
      "Tickets",
      "Audit log",
      "Notification settings",
      "Marketing",
      "Notifications",
      "Account",
      "Security",
      "Tesserix Support",
      "Privacy Policy",
      "Terms of Service",
    ]) {
      expect(getByText(label)).toBeTruthy();
    }
  });

  it("shows no unread badge when there are no unread notifications", () => {
    const { queryByText } = render(<MoreScreen />);
    expect(queryByText("0")).toBeNull();
  });

  it("shows the unread count pill and appends it to the row's accessibility label", () => {
    setUnread(3);
    const { getByText, getByLabelText } = render(<MoreScreen />);
    expect(getByText("3")).toBeTruthy();
    expect(getByLabelText(/Notifications inbox, 3 unread/)).toBeTruthy();
  });

  it("caps the unread pill at 99+", () => {
    setUnread(140);
    const { getByText } = render(<MoreScreen />);
    expect(getByText("99+")).toBeTruthy();
  });

  it("gives both Legal rows accessibilityRole=\"link\" — they leave the app via Linking", () => {
    const { getByLabelText } = render(<MoreScreen />);
    expect(getByLabelText("Privacy Policy — opens in your browser").props.accessibilityRole).toBe(
      "link",
    );
    expect(getByLabelText("Terms of Service — opens in your browser").props.accessibilityRole).toBe(
      "link",
    );
  });

  it("opens the live legal URLs via Linking, not in-app navigation", () => {
    const { getByLabelText } = render(<MoreScreen />);
    fireEvent.press(getByLabelText("Privacy Policy — opens in your browser"));
    expect(Linking.openURL).toHaveBeenCalledWith("https://mark8ly.com/privacy");
    fireEvent.press(getByLabelText("Terms of Service — opens in your browser"));
    expect(Linking.openURL).toHaveBeenCalledWith("https://mark8ly.com/terms");
  });

  it("navigates via router.push for an ordinary nav row", () => {
    const { getByText } = render(<MoreScreen />);
    fireEvent.press(getByText("Team"));
    expect(mockPush).toHaveBeenCalledWith("/(tabs)/more/settings/team");
  });

  // Screen gutter: theme.spacing.xl (20), matching theme.row.paddingH so the
  // eyebrow, card and rows share one left edge — the whole point of this
  // task's gutter normalisation, and something this screen already got
  // right pre-migration.
  it("keeps the screen body at the xl gutter", () => {
    const { UNSAFE_root } = render(<MoreScreen />);
    const { ScrollView } = require("react-native");
    const scroll = UNSAFE_root.findByType(ScrollView);
    const style = StyleSheet.flatten(scroll.props.contentContainerStyle);
    expect(style.paddingHorizontal).toBe(theme.spacing.xl);
  });
});
