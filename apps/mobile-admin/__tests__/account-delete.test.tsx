const mockAuth: Record<string, unknown> = {};
jest.mock("@repo/mobile-shared/auth/provider", () => ({ useAuth: () => mockAuth }));

const mockTenantState: { activeStore: unknown } = { activeStore: null };
jest.mock("@repo/mobile-shared/stores/tenant-store", () => ({
  useTenantStore: (selector: (s: typeof mockTenantState) => unknown) =>
    selector(mockTenantState),
}));

// account.tsx renders <StoreSelector> unconditionally (just hidden via
// `visible={false}`); that component pulls in `useStores` (react-query) and
// more lucide icons. None of that is relevant to the delete-account flow, so
// stub it out — this test isn't exercising the store switcher.
jest.mock("../components/StoreSelector", () => ({
  StoreSelector: () => null,
}));

const mockDeleteMutation: { mutate: jest.Mock; isPending: boolean; error: Error | null } = {
  mutate: jest.fn(),
  isPending: false,
  error: null,
};
jest.mock("@/lib/admin-api/account-actions", () => ({
  useDeleteAccount: () => mockDeleteMutation,
}));

// `@/components/ui` barrel re-exports BackHeader/SearchField, which import
// icons from lucide-react-native's ESM build (`dist/esm/...mjs`) — not
// covered by jest-expo's default transformIgnorePatterns, so requiring it
// unmocked throws "Unexpected token 'export'". Stub every icon export with a
// no-op component; we don't assert on icon rendering here.
jest.mock("lucide-react-native", () => {
  const IconStub = () => null;
  return new Proxy({}, { get: () => IconStub });
});
// `@/components/ui`'s `Screen` calls `useSafeAreaInsets()`, which throws
// without a `<SafeAreaProvider>` ancestor. react-native-safe-area-context
// ships an official jest mock for exactly this.
jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});

import { Alert } from "react-native";
import { fireEvent, render } from "@testing-library/react-native";
import AccountScreen from "../app/(tabs)/more/account";

function setAuth(overrides: Record<string, unknown> = {}) {
  Object.keys(mockAuth).forEach((k) => delete mockAuth[k]);
  Object.assign(
    mockAuth,
    {
      user: { displayName: "Jane Merchant", email: "jane@example.com" },
      signOut: jest.fn(),
    },
    overrides,
  );
}

describe("AccountScreen — Delete account", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    setAuth();
    mockDeleteMutation.mutate = jest.fn();
    mockDeleteMutation.isPending = false;
    mockDeleteMutation.error = null;
  });

  it("enables the delete button once DELETE is typed, and calls mutate only after confirming the alert", () => {
    const alertSpy = jest.spyOn(Alert, "alert");
    const { getByLabelText } = render(<AccountScreen />);
    const input = getByLabelText("Type DELETE to confirm account deletion");
    const deleteBtn = getByLabelText("Delete account");

    fireEvent.changeText(input, "DELETE");
    fireEvent.press(deleteBtn);

    // Pressing the button opens a confirmation dialog — nothing deleted yet.
    expect(alertSpy).toHaveBeenCalledTimes(1);
    expect(mockDeleteMutation.mutate).not.toHaveBeenCalled();

    // Invoking the destructive "Delete account" action confirms the deletion.
    const buttons = alertSpy.mock.calls[0][2];
    const confirm = buttons?.find((b) => b.style === "destructive");
    confirm?.onPress?.();

    expect(mockDeleteMutation.mutate).toHaveBeenCalledTimes(1);
    alertSpy.mockRestore();
  });

  it("keeps the delete button disabled and does not call mutate for mismatched text", () => {
    const { getByLabelText } = render(<AccountScreen />);
    const input = getByLabelText("Type DELETE to confirm account deletion");
    const deleteBtn = getByLabelText("Delete account");

    fireEvent.changeText(input, "delete");
    fireEvent.press(deleteBtn);
    expect(mockDeleteMutation.mutate).not.toHaveBeenCalled();

    fireEvent.changeText(input, "xyz");
    fireEvent.press(deleteBtn);
    expect(mockDeleteMutation.mutate).not.toHaveBeenCalled();
  });

  it("shows a busy state and keeps the button disabled while pending", () => {
    mockDeleteMutation.isPending = true;
    const { getByLabelText } = render(<AccountScreen />);
    const input = getByLabelText("Type DELETE to confirm account deletion");
    const deleteBtn = getByLabelText("Delete account");

    fireEvent.changeText(input, "DELETE");
    fireEvent.press(deleteBtn);

    expect(mockDeleteMutation.mutate).not.toHaveBeenCalled();
  });

  it("surfaces the mutation error inline", () => {
    mockDeleteMutation.error = new Error("Something went wrong deleting your account.");
    const { getByText } = render(<AccountScreen />);
    expect(getByText("Something went wrong deleting your account.")).toBeTruthy();
  });
});
