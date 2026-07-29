// Task 9: account.tsx's local `InfoRow` (a hand-rolled two-Text row) is
// replaced by a non-interactive GroupedRow, and the screen — which had NO
// ScrollView at all before this task — gets one, because a grouped list
// plus the danger zone can overflow a short device at raised text sizes.
const mockAuth: Record<string, unknown> = {};
jest.mock("@repo/mobile-shared/auth/provider", () => ({ useAuth: () => mockAuth }));

const mockTenantState: { activeStore: unknown } = {
  activeStore: { id: "s1", name: "Bondi Supply", slug: "bondi-supply" },
};
jest.mock("@repo/mobile-shared/stores/tenant-store", () => ({
  useTenantStore: (selector: (s: typeof mockTenantState) => unknown) => selector(mockTenantState),
}));

jest.mock("../components/StoreSelector", () => ({
  StoreSelector: () => null,
}));

jest.mock("@/lib/admin-api/account-actions", () => ({
  useDeleteAccount: () => ({ mutate: jest.fn(), isPending: false, error: null }),
}));

jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));
jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});

import { StyleSheet } from "react-native";
import { fireEvent, render } from "@testing-library/react-native";
import AccountScreen from "../app/(tabs)/more/account";
import { theme } from "@/lib/theme";

function setAuth(overrides: Record<string, unknown> = {}) {
  Object.keys(mockAuth).forEach((k) => delete mockAuth[k]);
  Object.assign(
    mockAuth,
    { user: { displayName: "Jane Merchant", email: "jane@example.com" }, signOut: jest.fn() },
    overrides,
  );
}

describe("AccountScreen — Profile / Store grouped rows", () => {
  beforeEach(() => setAuth());

  it("renders the profile fields as label + right-hand value", () => {
    const { getByText } = render(<AccountScreen />);
    expect(getByText("Name")).toBeTruthy();
    expect(getByText("Jane Merchant")).toBeTruthy();
    expect(getByText("Email")).toBeTruthy();
    expect(getByText("jane@example.com")).toBeTruthy();
  });

  it("falls back to placeholder copy for missing profile fields", () => {
    setAuth({ user: { displayName: null, email: undefined } });
    const { getByText } = render(<AccountScreen />);
    expect(getByText("Not set")).toBeTruthy();
    expect(getByText("—")).toBeTruthy();
  });

  // The interface's core trap: a row with no `onPress` must be a plain,
  // non-interactive View — not a disabled PressableRow, which announces to
  // VoiceOver as a dimmed button even though the merchant is only reading a
  // value here.
  it("renders the profile rows as non-interactive — no button role, no disabled state", () => {
    const { getByText } = render(<AccountScreen />);
    // Walk up from the value text to the row it's grouped inside.
    const valueNode = getByText("Jane Merchant");
    let node = valueNode.parent;
    while (node && node.props.accessibilityLabel === undefined) {
      node = node.parent;
    }
    expect(node).toBeTruthy();
    expect(node?.props.accessibilityRole).not.toBe("button");
    expect(node?.props.accessibilityState?.disabled).not.toBe(true);
  });

  it("keeps the Store row interactive and opens the store selector on press", () => {
    const { getByLabelText } = render(<AccountScreen />);
    const storeRow = getByLabelText("Current store: Bondi Supply. Tap to switch.");
    expect(storeRow.props.accessibilityRole).toBe("button");
    // Pressing it must not throw — StoreSelector is stubbed, so this only
    // proves the row's onPress is still wired, not the modal's contents.
    fireEvent.press(storeRow);
  });

  it("shows the store's slug alongside its name", () => {
    const { getByText } = render(<AccountScreen />);
    expect(getByText("bondi-supply")).toBeTruthy();
  });

  it("scrolls its content, with bottom clearance for the floating dock", () => {
    const { UNSAFE_root } = render(<AccountScreen />);
    const { ScrollView } = require("react-native");
    const scroll = UNSAFE_root.findByType(ScrollView);
    const style = StyleSheet.flatten(scroll.props.contentContainerStyle);
    // useDockClearance() = insets.bottom + DOCK_BOTTOM_GAP(4) + DOCK_HEIGHT(64) + 12.
    // The safe-area jest mock returns insets.bottom = 0, so this is the floor.
    expect(style.paddingBottom).toBeGreaterThanOrEqual(64 + 4 + 12);
  });

  it("keeps the Delete account danger zone inside the scrollable content, reachable above the dock", () => {
    const { getByLabelText, UNSAFE_root } = render(<AccountScreen />);
    const { ScrollView } = require("react-native");
    const scroll = UNSAFE_root.findByType(ScrollView);
    const deleteBtn = getByLabelText("Delete account");
    // Every ancestor from the delete button up to the ScrollView must exist
    // — i.e. the button is INSIDE the scroll tree, not a sibling the
    // ScrollView's clearance can't reach.
    const descendants = scroll.findAllByType(deleteBtn.type);
    expect(descendants).toContain(deleteBtn);
  });

  it("keeps the screen gutter at theme.spacing.xl", () => {
    const { UNSAFE_root } = render(<AccountScreen />);
    const { ScrollView } = require("react-native");
    const scroll = UNSAFE_root.findByType(ScrollView);
    const style = StyleSheet.flatten(scroll.props.contentContainerStyle);
    expect(style.paddingHorizontal).toBe(theme.spacing.xl);
  });
});
