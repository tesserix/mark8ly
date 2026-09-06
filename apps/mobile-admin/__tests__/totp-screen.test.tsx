// The authenticator-app code screen (#686 item 2).
//
// Its absence was a total lockout: a merchant with TOTP enrolled got
// `totp_required` from login, the client could not resume it, and the login
// screen said "this app version needs an update" — advice no update could
// satisfy.
//
// What is pinned here is what must NOT drift back toward /otp: the copy
// never mentions email, and a rejected code gets authenticator advice
// rather than the emailed screen's "sign in again to get a new code".
import { fireEvent, render, screen, waitFor } from "@testing-library/react-native";
import { ZitadelAuthError } from "@repo/mobile-shared/auth/zitadel-client";
import TotpScreen from "../app/totp";

const mockVerifyTotp = jest.fn();
const mockReplace = jest.fn();
let mockParams: Record<string, string> = {};

jest.mock("expo-router", () => ({
  router: { replace: (...args: unknown[]) => mockReplace(...args) },
  useLocalSearchParams: () => mockParams,
}));

jest.mock("@repo/mobile-shared/auth/zitadel-signin", () => ({
  createZitadelSignIn: () => ({ verifyTotp: mockVerifyTotp }),
}));

jest.mock("@repo/mobile-shared/config/env", () => ({
  useEnvironment: () => ({ apiBaseUrl: "https://api.mark8ly.test" }),
}));

jest.mock("@repo/mobile-shared/stores/tenant-store", () => ({
  useTenantStore: (selector: (s: unknown) => unknown) =>
    selector({ setTenantId: jest.fn() }),
}));

beforeEach(() => {
  mockVerifyTotp.mockReset().mockResolvedValue(undefined);
  mockReplace.mockReset();
  mockParams = { pendingToken: "pending-1" };
});

/** The library keeps one hidden TextInput behind the cells; type into that. */
function enterCode(code: string) {
  fireEvent.changeText(screen.getByLabelText("Authenticator code"), code);
}

it("verifies with the whole code the moment the last cell fills", async () => {
  render(<TotpScreen />);

  enterCode("224365");

  await waitFor(() =>
    expect(mockVerifyTotp).toHaveBeenCalledWith(
      "pending-1",
      "224365",
      expect.anything(),
    ),
  );
  await waitFor(() => expect(mockReplace).toHaveBeenCalledWith("/(tabs)"));
});

it("does not submit a partial code", async () => {
  render(<TotpScreen />);

  enterCode("2243");

  await waitFor(() => expect(mockVerifyTotp).not.toHaveBeenCalled());
});

// Sending someone to their inbox for a code that only exists inside an
// authenticator app is the defect this screen was split off to avoid.
it("never tells the merchant to check their email", () => {
  render(<TotpScreen />);

  expect(screen.queryByText(/email/i)).toBeNull();
  expect(screen.getByText(/authenticator app/i)).toBeTruthy();
});

it("shows authenticator copy for a rejected code, not password copy", async () => {
  mockVerifyTotp.mockRejectedValue(new ZitadelAuthError("invalid_totp", ""));
  render(<TotpScreen />);

  enterCode("000000");

  await waitFor(() =>
    expect(screen.getByText(/wait for the next code/i)).toBeTruthy(),
  );
  expect(screen.queryByText(/check your details/i)).toBeNull();
});

// A correct code entered during an outage must not be reported as wrong, or
// the merchant retypes a rolling six-digit code forever.
it("reports an outage as an outage", async () => {
  mockVerifyTotp.mockRejectedValue(new ZitadelAuthError("auth_unavailable", ""));
  render(<TotpScreen />);

  enterCode("224365");

  await waitFor(() =>
    expect(screen.getByText(/temporarily unavailable/i)).toBeTruthy(),
  );
});

it("refuses to submit when the sign-in attempt carries no pending token", async () => {
  mockParams = {};
  render(<TotpScreen />);

  enterCode("224365");

  await waitFor(() =>
    expect(screen.getByText(/sign-in attempt has expired/i)).toBeTruthy(),
  );
  expect(mockVerifyTotp).not.toHaveBeenCalled();
});
