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
  const authFn: any = () => ({ signInWithCredential });
  authFn.GoogleAuthProvider = { credential: googleCredential };
  authFn.AppleAuthProvider = { credential: appleCredential };
  return { __esModule: true, default: authFn };
});

import auth from "@react-native-firebase/auth";
import { signInWithGoogleCredential, signInWithAppleCredential } from "@repo/mobile-shared/auth/social-credentials";

const mockedAuth = auth as unknown as {
  (): { signInWithCredential: jest.Mock };
  GoogleAuthProvider: { credential: jest.Mock };
  AppleAuthProvider: { credential: jest.Mock };
};

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

it("maps an account-exists conflict to needs-link with the pending credential (google)", async () => {
  const conflict = Object.assign(
    new Error("account exists"),
    { code: "auth/account-exists-with-different-credential", email: "merchant@store.com" },
  );
  mockedAuth().signInWithCredential.mockRejectedValueOnce(conflict);

  const outcome = await signInWithGoogleCredential("gtok");

  expect(outcome).toEqual({
    status: "needs-link",
    email: "merchant@store.com",
    provider: "google.com",
    pendingCredential: { provider: "google", idToken: "gtok" },
  });
});

it("maps an account-exists conflict to needs-link (apple)", async () => {
  const conflict = Object.assign(
    new Error("account exists"),
    { code: "auth/account-exists-with-different-credential", email: "merchant@store.com" },
  );
  mockedAuth().signInWithCredential.mockRejectedValueOnce(conflict);

  const outcome = await signInWithAppleCredential("atok", "nonce123", null);

  expect(outcome).toEqual({
    status: "needs-link",
    email: "merchant@store.com",
    provider: "apple.com",
    pendingCredential: { provider: "apple", idToken: "atok", nonce: "nonce123" },
  });
});

it("rethrows non-conflict errors", async () => {
  mockedAuth().signInWithCredential.mockRejectedValueOnce(
    Object.assign(new Error("network"), { code: "auth/network-request-failed" }),
  );
  await expect(signInWithGoogleCredential("gtok")).rejects.toThrow("network");
});
