// Mocks are built INSIDE the jest.mock factory (babel hoists imports above
// outer const/var) and read back off the imported `auth` default export.
jest.mock("@react-native-firebase/auth", () => {
  const linkWithCredential = jest.fn().mockResolvedValue(undefined);
  const user = { linkWithCredential };
  const instance = {
    signInWithEmailAndPassword: jest.fn().mockResolvedValue({ user }),
    signInWithCredential: jest.fn().mockResolvedValue({ user }),
    fetchSignInMethodsForEmail: jest.fn().mockResolvedValue(["password"]),
  };
  const authFn = () => instance;
  authFn.GoogleAuthProvider = {
    credential: jest.fn((idToken: string) => ({ provider: "google", idToken })),
  };
  authFn.AppleAuthProvider = {
    credential: jest.fn((idToken: string, nonce: string) => ({
      provider: "apple",
      idToken,
      nonce,
    })),
  };
  return { __esModule: true, default: authFn };
});

import auth from "@react-native-firebase/auth";
import {
  completeLinkWithPassword,
  completeLinkWithGoogle,
  completeLinkWithApple,
  existingSignInMethods,
} from "@repo/mobile-shared/auth/link";

interface MockedInstance {
  signInWithEmailAndPassword: jest.Mock;
  signInWithCredential: jest.Mock;
  fetchSignInMethodsForEmail: jest.Mock;
}
const mockedAuth = auth as unknown as (() => MockedInstance) & {
  GoogleAuthProvider: { credential: jest.Mock };
  AppleAuthProvider: { credential: jest.Mock };
};
const PENDING = { provider: "google", idToken: "pending-tok" } as never;

describe("account linking", () => {
  beforeEach(() => jest.clearAllMocks());

  it("password re-auth signs in then links the pending credential", async () => {
    await completeLinkWithPassword("merchant@store.com", "pw", PENDING);
    const instance = mockedAuth();
    expect(instance.signInWithEmailAndPassword).toHaveBeenCalledWith(
      "merchant@store.com",
      "pw",
    );
    const linked = await instance.signInWithEmailAndPassword.mock.results[0]!.value;
    expect(linked.user.linkWithCredential).toHaveBeenCalledWith(PENDING);
  });

  it("google re-auth builds the existing credential, signs in, then links", async () => {
    await completeLinkWithGoogle("existing-gtok", PENDING);
    expect(mockedAuth.GoogleAuthProvider.credential).toHaveBeenCalledWith("existing-gtok");
    const instance = mockedAuth();
    expect(instance.signInWithCredential).toHaveBeenCalledWith({
      provider: "google",
      idToken: "existing-gtok",
    });
    const linked = await instance.signInWithCredential.mock.results[0]!.value;
    expect(linked.user.linkWithCredential).toHaveBeenCalledWith(PENDING);
  });

  it("apple re-auth builds the existing credential, signs in, then links", async () => {
    await completeLinkWithApple("existing-atok", "nonce", PENDING);
    expect(mockedAuth.AppleAuthProvider.credential).toHaveBeenCalledWith(
      "existing-atok",
      "nonce",
    );
    const instance = mockedAuth();
    expect(instance.signInWithCredential).toHaveBeenCalledWith({
      provider: "apple",
      idToken: "existing-atok",
      nonce: "nonce",
    });
    const linked = await instance.signInWithCredential.mock.results[0]!.value;
    expect(linked.user.linkWithCredential).toHaveBeenCalledWith(PENDING);
  });

  it("existingSignInMethods returns the native result", async () => {
    await expect(existingSignInMethods("merchant@store.com")).resolves.toEqual(["password"]);
    expect(mockedAuth().fetchSignInMethodsForEmail).toHaveBeenCalledWith("merchant@store.com");
  });

  it("existingSignInMethods returns [] when enumeration protection blocks it", async () => {
    mockedAuth().fetchSignInMethodsForEmail.mockResolvedValueOnce([]);
    await expect(existingSignInMethods("merchant@store.com")).resolves.toEqual([]);
  });
});
