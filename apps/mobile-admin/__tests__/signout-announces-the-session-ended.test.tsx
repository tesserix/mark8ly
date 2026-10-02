// Signing out must CHANGE SOMETHING a route guard can watch.
//
// Reported from a device: "sign out doesn't go back to the login screen unless
// back is pressed". Under Zitadel `user` is always null — that field belongs to
// the Firebase SDK — so AuthGate decides signed-in-ness from a token read it
// runs in an effect keyed on the route. Signing out clears the tokens and
// changes no route, so that effect never re-ran: the guard kept its stale
// answer and never redirected. Pressing Back changed the route, which is the
// only reason Back appeared to "work".
//
// `lib/api-client.ts` signs out the same way on a 401, so an expired session
// was stranded on whatever screen it died on.
//
// The fix is `sessionEpoch`, which AuthGate now depends on alongside the
// route. This asserts the provider advances it, and that by the time it does,
// the tokens are already gone — a guard that re-read a still-present token
// would send the user straight back in.
import React from "react";
import { Text } from "react-native";
import { render, screen, act } from "@testing-library/react-native";
import { AuthProvider, useAuth } from "@repo/mobile-shared/auth/provider";
import { zitadelSession } from "@repo/mobile-shared/auth/zitadel-session";

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

/**
 * Renders the epoch so the test can read it the way AuthGate consumes it — as
 * a value that changes — and hands `signOut` out so the test can drive it
 * inside `act` rather than from an effect.
 */
let doSignOut: () => Promise<void>;
function Harness() {
  const { signOut, sessionEpoch } = useAuth();
  doSignOut = signOut;
  return <Text>{`epoch:${sessionEpoch}`}</Text>;
}

it("advances sessionEpoch on sign-out, with the tokens already cleared", async () => {
  await zitadelSession.save("AT", "RT", 3600);

  render(
    <AuthProvider>
      <Harness />
    </AuthProvider>,
  );
  await screen.findByText("epoch:0");

  await act(async () => {
    await doSignOut();
  });

  // Without the epoch this stays "epoch:0" for the life of the session, and
  // nothing ever tells AuthGate that anything happened.
  await screen.findByText("epoch:1");
  expect(await zitadelSession.read()).toBeNull();
});
