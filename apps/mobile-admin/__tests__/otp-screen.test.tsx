// The emailed-code screen (#686).
//
// The auto-submit path is the one worth pinning: CodeInput reports the full
// code the instant its last cell fills, and reading component state there
// instead of that argument would send a FIVE-digit code — a wrong-code error
// on a code the merchant typed correctly, inside a challenge that expires in
// five minutes.
import { fireEvent, render, screen, waitFor } from "@testing-library/react-native";
import OtpScreen from "../app/otp";

const mockVerifyOtp = jest.fn();
const mockReplace = jest.fn();
let mockParams: Record<string, string> = {};

jest.mock("expo-router", () => ({
  router: { replace: (...args: unknown[]) => mockReplace(...args) },
  useLocalSearchParams: () => mockParams,
}));

jest.mock("@repo/mobile-shared/auth/zitadel-signin", () => ({
  createZitadelSignIn: () => ({ verifyOtp: mockVerifyOtp }),
}));

jest.mock("@repo/mobile-shared/config/env", () => ({
  useEnvironment: () => ({ apiBaseUrl: "https://api.mark8ly.test" }),
}));

jest.mock("@repo/mobile-shared/stores/tenant-store", () => ({
  useTenantStore: (selector: (s: unknown) => unknown) =>
    selector({ setTenantId: jest.fn() }),
}));

beforeEach(() => {
  mockVerifyOtp.mockReset().mockResolvedValue(undefined);
  mockReplace.mockReset();
  mockParams = { pendingToken: "pending-1", email: "demo@mark8ly.test" };
});

/** The library keeps one hidden TextInput behind the cells; type into that. */
function enterCode(code: string) {
  fireEvent.changeText(screen.getByLabelText("Email code"), code);
}

it("verifies with the whole code the moment the last cell fills", async () => {
  render(<OtpScreen />);

  enterCode("224365");

  await waitFor(() =>
    expect(mockVerifyOtp).toHaveBeenCalledWith(
      "pending-1",
      "224365",
      expect.anything(),
    ),
  );
  await waitFor(() => expect(mockReplace).toHaveBeenCalledWith("/(tabs)"));
});

it("does not submit a partial code", async () => {
  render(<OtpScreen />);

  enterCode("2243");

  await waitFor(() => expect(mockVerifyOtp).not.toHaveBeenCalled());
});

// Pressable hands its press event to the first argument, so a bare
// `onPress={handleVerify}` would submit a GestureResponderEvent as the code.
it("sends the typed code, not the press event, when Verify is tapped", async () => {
  render(<OtpScreen />);

  enterCode("2243");
  fireEvent.press(screen.getByLabelText("Verify code"));

  await waitFor(() =>
    expect(mockVerifyOtp).toHaveBeenCalledWith(
      "pending-1",
      "2243",
      expect.anything(),
    ),
  );
});

it("refuses to submit when the sign-in attempt carries no pending token", async () => {
  mockParams = { email: "demo@mark8ly.test" };
  render(<OtpScreen />);

  enterCode("224365");

  await waitFor(() =>
    expect(
      screen.getByText(/sign-in attempt has expired/i),
    ).toBeTruthy(),
  );
  expect(mockVerifyOtp).not.toHaveBeenCalled();
});
