// NOTE: the mock functions are created *inside* the jest.mock() factory and
// read back off the imported `auth` default export (instead of being
// declared as outer `const`s and captured by the factory closure). This
// repo's babel/jest preset downlevels `const`/`let` to `var`, so outer
// "mock"-prefixed bindings referenced inside a jest.mock() factory are only
// hoisted (not yet assigned) at the moment the factory runs — they'd read as
// `undefined`. Building the mocks inside the factory sidesteps that.
jest.mock("@react-native-firebase/auth", () => {
  const signInWithCredential = jest.fn().mockResolvedValue({ user: { displayName: "X", updateProfile: jest.fn() } });
  const googleCredential = jest.fn((idToken: string, accessToken?: string) => ({ provider: "google", idToken }));
  const appleCredential = jest.fn((idToken: string, nonce: string) => ({ provider: "apple", idToken, nonce }));
  return {
    __esModule: true,
    getAuth: () => ({ signInWithCredential }),
    GoogleAuthProvider: { credential: googleCredential },
    AppleAuthProvider: { credential: appleCredential },
  };
});

import { fromByteArray } from "base64-js";
import {
  getAuth,
  GoogleAuthProvider,
  AppleAuthProvider,
} from "@react-native-firebase/auth";
import { signInWithGoogleCredential, signInWithAppleCredential } from "@repo/mobile-shared/auth/social-credentials";

const mockedAuth = Object.assign(
  getAuth as unknown as () => { signInWithCredential: jest.Mock },
  {
    GoogleAuthProvider: GoogleAuthProvider as unknown as { credential: jest.Mock },
    AppleAuthProvider: AppleAuthProvider as unknown as { credential: jest.Mock },
  },
);

// Builds a syntactically valid (unsigned) JWT carrying the given claims, so
// the id_token decoding path under test can be exercised end-to-end without
// a real Firebase/Google/Apple token.
function makeIdToken(claims: Record<string, unknown>): string {
  const json = JSON.stringify(claims);
  const bytes = Uint8Array.from(Array.from(json).map((c) => c.charCodeAt(0)));
  const b64url = fromByteArray(bytes).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  return `header.${b64url}.sig`;
}

// This is the REAL React Native Firebase error shape: `code` + `message` +
// `userInfo`. Critically, there is NO `email` property — RNFirebase never
// puts one on the rejection. A fixture that invented `email` would hide the
// exact bug this suite guards against.
function accountExistsConflict(userInfo?: Record<string, unknown>) {
  return Object.assign(new Error("account exists"), {
    code: "auth/account-exists-with-different-credential",
    nativeErrorMessage: "account exists with different credential",
    userInfo,
  });
}

it("maps a Google id_token to a GIP credential sign-in", async () => {
  await signInWithGoogleCredential("gtok");
  expect(mockedAuth.GoogleAuthProvider.credential).toHaveBeenCalledWith("gtok", undefined);
  expect(mockedAuth().signInWithCredential).toHaveBeenCalledWith({ provider: "google", idToken: "gtok" });
});

it("maps an Apple id_token + nonce to a GIP credential sign-in", async () => {
  await signInWithAppleCredential("atok", "nonce123", null);
  expect(mockedAuth.AppleAuthProvider.credential).toHaveBeenCalledWith("atok", "nonce123");
  expect(mockedAuth().signInWithCredential).toHaveBeenCalledWith({ provider: "apple", idToken: "atok", nonce: "nonce123" });
});

it("returns signed-in when the credential sign-in succeeds", async () => {
  const outcome = await signInWithGoogleCredential("gtok");
  expect(outcome).toEqual({ status: "signed-in" });
});

it("maps an account-exists conflict to needs-link, deriving email from the id_token claim (google)", async () => {
  const gtok = makeIdToken({ email: "merchant@store.com" });
  mockedAuth().signInWithCredential.mockRejectedValueOnce(accountExistsConflict());

  const outcome = await signInWithGoogleCredential(gtok);

  expect(outcome).toEqual({
    status: "needs-link",
    email: "merchant@store.com",
    provider: "google.com",
    pendingCredential: { provider: "google", idToken: gtok },
  });
});

it("maps an account-exists conflict to needs-link, deriving email from the id_token claim (apple)", async () => {
  const atok = makeIdToken({ email: "merchant@store.com" });
  mockedAuth().signInWithCredential.mockRejectedValueOnce(accountExistsConflict());

  const outcome = await signInWithAppleCredential(atok, "nonce123", null);

  expect(outcome).toEqual({
    status: "needs-link",
    email: "merchant@store.com",
    provider: "apple.com",
    pendingCredential: { provider: "apple", idToken: atok, nonce: "nonce123" },
  });
});

it("falls back to userInfo.email when the id_token carries no email claim", async () => {
  const gtok = makeIdToken({ sub: "12345" });
  mockedAuth().signInWithCredential.mockRejectedValueOnce(
    accountExistsConflict({ email: "fallback@store.com" }),
  );

  const outcome = await signInWithGoogleCredential(gtok);

  expect(outcome).toEqual({
    status: "needs-link",
    email: "fallback@store.com",
    provider: "google.com",
    pendingCredential: { provider: "google", idToken: gtok },
  });
});

it("resolves to an empty email (not a throw) when neither the id_token nor userInfo carries one", async () => {
  const gtok = makeIdToken({ sub: "12345" });
  mockedAuth().signInWithCredential.mockRejectedValueOnce(accountExistsConflict());

  const outcome = await signInWithGoogleCredential(gtok);

  expect(outcome).toEqual({
    status: "needs-link",
    email: "",
    provider: "google.com",
    pendingCredential: { provider: "google", idToken: gtok },
  });
});

it("rethrows non-conflict errors", async () => {
  mockedAuth().signInWithCredential.mockRejectedValueOnce(
    Object.assign(new Error("network"), { code: "auth/network-request-failed" }),
  );
  await expect(signInWithGoogleCredential("gtok")).rejects.toThrow("network");
});
