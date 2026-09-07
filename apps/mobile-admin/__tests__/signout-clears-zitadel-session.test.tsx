// Signing out must actually sign the user out on a Zitadel build.
//
// The Zitadel bearer tokens live in SecureStore, owned by this app rather than
// by an auth SDK, so `backend.signOut()` cannot reach them. Without an
// explicit clear, "Sign Out" wiped the tenant/store ids and left the tokens
// behind: AuthGate re-read a still-fresh access token, concluded the user was
// signed in, and replaced /login with the dashboard. The 401 sign-out path
// (api-client's onUnauthorized) landed in the same place.
import React from "react";
import { Text } from "react-native";
import { render, screen, waitFor } from "@testing-library/react-native";
import { AuthProvider, useAuth } from "@repo/mobile-shared/auth/provider";
import { zitadelSession } from "@repo/mobile-shared/auth/zitadel-session";
import { tokenStorage } from "@repo/mobile-shared/auth/token-storage";

// expo-constants pulls in `expo/virtual/env`, which does not resolve under
// jest. Reporting Expo Go also selects the demo backend, which is what this
// test wants: the clear under test lives in the provider, above the backend.
jest.mock("expo-constants", () => ({
  __esModule: true,
  default: { executionEnvironment: "storeClient", expoConfig: { extra: {} } },
  ExecutionEnvironment: { StoreClient: "storeClient", Standalone: "standalone", Bare: "bare" },
}));

jest.mock("expo-secure-store", () => {
  const mem: Record<string, string> = {};
  return {
    __mem: mem,
    getItemAsync: jest.fn(async (k: string) => mem[k] ?? null),
    setItemAsync: jest.fn(async (k: string, v: string) => {
      mem[k] = v;
    }),
    deleteItemAsync: jest.fn(async (k: string) => {
      delete mem[k];
    }),
  };
});

const mem = (jest.requireMock("expo-secure-store") as { __mem: Record<string, string> }).__mem;

beforeEach(() => {
  process.env.EXPO_PUBLIC_AUTH_BACKEND = "demo";
  for (const k of Object.keys(mem)) delete mem[k];
});
afterEach(() => {
  delete process.env.EXPO_PUBLIC_AUTH_BACKEND;
});

function SignOutOnMount() {
  const { signOut } = useAuth();
  React.useEffect(() => {
    void signOut();
  }, [signOut]);
  return <Text>ready</Text>;
}

it("clears the persisted Zitadel tokens, not just the tenant ids", async () => {
  await zitadelSession.save("AT", "RT", 3600);
  await tokenStorage.setTenantId("t-1");

  render(
    <AuthProvider>
      <SignOutOnMount />
    </AuthProvider>,
  );
  await screen.findByText("ready");

  await waitFor(async () => {
    expect(await zitadelSession.read()).toBeNull();
  });
  // The pre-existing behaviour still holds.
  expect(await tokenStorage.getTenantId()).toBeNull();
});
