// Task 9: team/index.tsx's member/invitation rows move off a plain-View
// list (no Card at all) onto GroupedList/GroupedRow. The owner row's
// non-interactive treatment is the one assertion the task's Step 1 names
// explicitly for this screen — it must survive the migration, and now reads
// as a genuinely non-interactive row (no `onPress`) rather than a disabled
// PressableRow.
jest.mock("lucide-react-native", () => new Proxy({}, { get: () => () => null }));
jest.mock("react-native-safe-area-context", () => {
  const mock = require("react-native-safe-area-context/jest/mock");
  return { __esModule: true, ...mock.default };
});
jest.mock("expo-router", () => ({ useRouter: () => ({ push: jest.fn() }) }));

const mockAlert = jest.fn(
  (_t: string, _m: string, buttons?: { style?: string; onPress?: () => void }[]) => {
    const confirm = (buttons ?? []).find((b) => b.style === "destructive");
    confirm?.onPress?.();
  },
);
jest.mock("react-native/Libraries/Alert/Alert", () => ({
  default: { alert: (...args: unknown[]) => (mockAlert as (...a: unknown[]) => void)(...args) },
}));

type Member = { email: string; role: string; kind?: "owner" | "member" };
type Invitation = { id: string; email: string; role: string; status: string };

let mockMembers: {
  list: Member[];
  isLoading: boolean;
  isError: boolean;
  isRefetching: boolean;
  refetch: jest.Mock;
};
let mockInvitations: {
  list: Invitation[];
  isLoading: boolean;
  isError: boolean;
  isRefetching: boolean;
  refetch: jest.Mock;
};
const mockUpdateRole = { mutate: jest.fn(), isPending: false };
const mockRevoke = { mutate: jest.fn(), isPending: false };

jest.mock("@/lib/hooks/use-team", () => ({
  useTeamMembers: () => ({
    data: { data: mockMembers.list },
    isLoading: mockMembers.isLoading,
    isError: mockMembers.isError,
    isRefetching: mockMembers.isRefetching,
    refetch: mockMembers.refetch,
  }),
  useTeamInvitations: () => ({
    data: { data: mockInvitations.list },
    isLoading: mockInvitations.isLoading,
    isError: mockInvitations.isError,
    isRefetching: mockInvitations.isRefetching,
    refetch: mockInvitations.refetch,
  }),
}));
jest.mock("@/lib/admin-api/team-actions", () => ({
  useUpdateMemberRole: () => mockUpdateRole,
  useRevokeInvitation: () => mockRevoke,
}));

import { StyleSheet } from "react-native";
import { fireEvent, render } from "@testing-library/react-native";
import TeamScreen from "../app/(tabs)/more/settings/team/index";
import { theme } from "@/lib/theme";

function reset() {
  mockMembers = {
    list: [
      { email: "owner@bondi.test", role: "owner", kind: "owner" },
      { email: "staff@bondi.test", role: "staff", kind: "member" },
    ],
    isLoading: false,
    isError: false,
    isRefetching: false,
    refetch: jest.fn(),
  };
  mockInvitations = {
    list: [{ id: "inv1", email: "pending@bondi.test", role: "staff", status: "pending" }],
    isLoading: false,
    isError: false,
    isRefetching: false,
    refetch: jest.fn(),
  };
  mockUpdateRole.mutate = jest.fn();
  mockUpdateRole.isPending = false;
  mockRevoke.mutate = jest.fn();
  mockRevoke.isPending = false;
  mockAlert.mockClear();
}

describe("TeamScreen — grouped rows", () => {
  beforeEach(reset);

  it("renders member rows with their role StatusBadge in trailing", () => {
    const { getByText } = render(<TeamScreen />);
    expect(getByText("owner@bondi.test")).toBeTruthy();
    expect(getByText("staff@bondi.test")).toBeTruthy();
    expect(getByText("Owner")).toBeTruthy();
    expect(getByText("Staff")).toBeTruthy();
  });

  // The Step 1 guard named explicitly in the task-9 brief.
  it("keeps the owner row non-interactive — no button role, no press effect", () => {
    const { getByLabelText } = render(<TeamScreen />);
    const ownerRow = getByLabelText("owner@bondi.test, owner");
    expect(ownerRow.props.accessibilityRole).not.toBe("button");
    expect(ownerRow.props.accessibilityState?.disabled).not.toBe(true);
    fireEvent.press(ownerRow);
    expect(mockAlert).not.toHaveBeenCalled();
  });

  it("lets a tap on a non-owner member open the role-change alert", () => {
    const { getByLabelText } = render(<TeamScreen />);
    const staffRow = getByLabelText("staff@bondi.test, staff. Tap to change role");
    expect(staffRow.props.accessibilityRole).toBe("button");
    fireEvent.press(staffRow);
    expect(mockAlert).toHaveBeenCalledWith("Change role", "staff@bondi.test", expect.anything());
  });

  it("makes every member row non-interactive while a role change is in flight", () => {
    mockUpdateRole.isPending = true;
    const { getByLabelText } = render(<TeamScreen />);
    const staffRow = getByLabelText("staff@bondi.test, staff");
    expect(staffRow.props.accessibilityRole).not.toBe("button");
    fireEvent.press(staffRow);
    expect(mockAlert).not.toHaveBeenCalled();
  });

  it("renders invitation rows as non-interactive, with role/status as a hint and Revoke in trailing", () => {
    const { getByText, getByLabelText } = render(<TeamScreen />);
    expect(getByText("pending@bondi.test")).toBeTruthy();
    expect(getByText("Staff · Pending")).toBeTruthy();
    const inviteRow = getByLabelText("pending@bondi.test");
    expect(inviteRow.props.accessibilityRole).not.toBe("button");
    const revokeBtn = getByLabelText("Revoke invite to pending@bondi.test");
    fireEvent.press(revokeBtn);
    expect(mockAlert).toHaveBeenCalledWith(
      "Revoke invitation",
      "Cancel the invite to pending@bondi.test?",
      expect.anything(),
    );
    expect(mockRevoke.mutate).toHaveBeenCalledWith("inv1", expect.anything());
  });

  it("falls back to plain copy, not an empty Card, when there are no invitations", () => {
    mockInvitations.list = [];
    const { getByText, queryByText } = render(<TeamScreen />);
    expect(getByText("No pending invitations.")).toBeTruthy();
    expect(queryByText("pending@bondi.test")).toBeNull();
  });

  it("gives a member row the app-wide 64pt minHeight, never a fixed height", () => {
    const { getByLabelText } = render(<TeamScreen />);
    const staffRow = getByLabelText("staff@bondi.test, staff. Tap to change role");
    const style = StyleSheet.flatten(staffRow.props.style);
    expect(style.minHeight).toBe(theme.row.minHeightSingle);
    expect(style.height).toBeUndefined();
  });

  it("keeps the screen gutter at theme.spacing.xl", () => {
    const { UNSAFE_root } = render(<TeamScreen />);
    const { ScrollView } = require("react-native");
    const scroll = UNSAFE_root.findByType(ScrollView);
    const style = StyleSheet.flatten(scroll.props.contentContainerStyle);
    expect(style.paddingHorizontal).toBe(theme.spacing.xl);
  });
});
