// The emailed-code screen (#686).
//
// The auto-submit path is the one worth pinning: CodeInput reports the full
// code the instant its last cell fills, and reading component state there
// instead of that argument would send a FIVE-digit code — a wrong-code error
// on a code the merchant typed correctly, inside a challenge that expires in
// five minutes.
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react-native";
import OtpScreen from "../app/otp";
import { ZitadelAuthError } from "@repo/mobile-shared/auth/zitadel-client";

const mockVerifyOtp = jest.fn();
const mockResendOtp = jest.fn();
const mockReplace = jest.fn();
let mockParams: Record<string, string> = {};

jest.mock("expo-router", () => ({
  router: { replace: (...args: unknown[]) => mockReplace(...args) },
  useLocalSearchParams: () => mockParams,
}));

jest.mock("@repo/mobile-shared/auth/zitadel-signin", () => ({
  createZitadelSignIn: () => ({ verifyOtp: mockVerifyOtp, resendOtp: mockResendOtp }),
}));

jest.mock("@repo/mobile-shared/config/env", () => ({
  useEnvironment: () => ({ apiBaseUrl: "https://api.mark8ly.test" }),
}));

jest.mock("@repo/mobile-shared/stores/tenant-store", () => ({
  useTenantStore: (selector: (s: unknown) => unknown) =>
    selector({ setTenantId: jest.fn() }),
}));

beforeEach(() => {
  jest.useFakeTimers();
  mockVerifyOtp.mockReset().mockResolvedValue(undefined);
  mockResendOtp.mockReset().mockResolvedValue("resealed-2");
  mockReplace.mockReset();
  mockParams = { pendingToken: "pending-1", email: "demo@mark8ly.test" };
});

afterEach(() => {
  jest.useRealTimers();
});

/**
 * The resend button is disabled for a 30-second cooldown from mount, so
 * every resend test has to get past it first.
 */
async function runOutTheCooldown() {
  await act(async () => {
    jest.advanceTimersByTime(31_000);
  });
}

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

// THE regression this feature can introduce. A resend re-seals the
// challenge and returns a NEW pending token; if the screen kept reading the
// route param, the next verify would submit the stale half of the pair and
// the merchant would be told a correct code was wrong — the exact failure
// the resend exists to prevent.
it("verifies with the pending token the resend returned, not the original", async () => {
  render(<OtpScreen />);
  await runOutTheCooldown();

  fireEvent.press(screen.getByLabelText("Resend code"));
  await waitFor(() => expect(mockResendOtp).toHaveBeenCalledWith("pending-1"));

  enterCode("224365");

  await waitFor(() =>
    expect(mockVerifyOtp).toHaveBeenCalledWith(
      "resealed-2",
      "224365",
      expect.anything(),
    ),
  );
  expect(mockVerifyOtp).not.toHaveBeenCalledWith(
    "pending-1",
    expect.anything(),
    expect.anything(),
  );
});

// The cooldown exists so a merchant cannot burn the whole 15-minute code
// budget in under a minute and then face a wall with no usable code.
it("disables resend during the cooldown and shows the remaining seconds", async () => {
  render(<OtpScreen />);

  expect(screen.getByText(/Resend in \d+s/)).toBeTruthy();
  fireEvent.press(screen.getByLabelText("Resend code"));
  expect(mockResendOtp).not.toHaveBeenCalled();

  await runOutTheCooldown();

  expect(screen.getByText("Resend code")).toBeTruthy();
  fireEvent.press(screen.getByLabelText("Resend code"));
  await waitFor(() => expect(mockResendOtp).toHaveBeenCalled());
});

// The cooldown restarts after a successful resend, for the same reason.
it("restarts the cooldown after a successful resend", async () => {
  render(<OtpScreen />);
  await runOutTheCooldown();

  fireEvent.press(screen.getByLabelText("Resend code"));
  await waitFor(() => expect(screen.getByText("We sent a new code.")).toBeTruthy());

  expect(screen.getByText(/Resend in \d+s/)).toBeTruthy();
});

// A spent budget gets its own copy: "wait a few minutes" is the only thing
// that helps, and generic retry copy would send the merchant round a loop
// that cannot end.
it("tells the merchant to wait when the code budget is spent", async () => {
  mockResendOtp.mockRejectedValue(new ZitadelAuthError("rate_limited", ""));
  render(<OtpScreen />);
  await runOutTheCooldown();

  fireEvent.press(screen.getByLabelText("Resend code"));

  await waitFor(() =>
    expect(screen.getByText(/too many codes/i)).toBeTruthy(),
  );
});

// A stale "that code isn't right" under a brand new code is a verdict on a
// code that no longer exists.
it("clears a stale error and confirms plainly on a successful resend", async () => {
  mockVerifyOtp.mockRejectedValue(new ZitadelAuthError("invalid_code", ""));
  render(<OtpScreen />);

  enterCode("224365");
  await waitFor(() => expect(screen.getByText(/isn't right/i)).toBeTruthy());

  await runOutTheCooldown();
  fireEvent.press(screen.getByLabelText("Resend code"));

  await waitFor(() => expect(screen.getByText("We sent a new code.")).toBeTruthy());
  expect(screen.queryByText(/isn't right/i)).toBeNull();
});

// The old copy sent people back to sign in again, which is no longer the
// only way to get a fresh code.
it("points a rejected code at the Resend button, not at signing in again", async () => {
  mockVerifyOtp.mockRejectedValue(new ZitadelAuthError("invalid_code", ""));
  render(<OtpScreen />);

  enterCode("224365");

  await waitFor(() => expect(screen.getByText(/Tap Resend/i)).toBeTruthy());
  expect(screen.queryByText(/Sign in again to get a new code/i)).toBeNull();
});
