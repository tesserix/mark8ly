// Mocks are built INSIDE the jest.mock factory (babel hoists imports above
// outer const/var) and read back off the imported `auth` default export.
jest.mock("@react-native-firebase/auth", () => {
  const linkWithCredential = jest.fn().mockResolvedValue(undefined);
  const user = { linkWithCredential };
  const instance = {
    signInWithEmailAndPassword: jest.fn().mockResolvedValue({ user }),
    signInWithCredential: jest.fn().mockResolvedValue({ user }),
    fetchSignInMethodsForEmail: jest.fn().mockResolvedValue(["password"]),
    currentUser: null as unknown,
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
  linkedProviderIds,
  linkGoogleToCurrentUser,
  linkAppleToCurrentUser,
  unlinkProvider,
  LastSignInMethodError,
} from "@repo/mobile-shared/auth/link";

interface MockedCurrentUser {
  providerData: { providerId: string }[];
  linkWithCredential: jest.Mock;
  unlink: jest.Mock;
}
interface MockedInstance {
  signInWithEmailAndPassword: jest.Mock;
  signInWithCredential: jest.Mock;
  fetchSignInMethodsForEmail: jest.Mock;
  currentUser: MockedCurrentUser | null;
}
const mockedAuth = auth as unknown as (() => MockedInstance) & {
  GoogleAuthProvider: { credential: jest.Mock };
  AppleAuthProvider: { credential: jest.Mock };
};
const PENDING = { provider: "google", idToken: "pending-tok" } as never;

/** Assigns a fresh mock currentUser (or null) onto the mocked auth instance. */
function setCurrentUser(providerData: { providerId: string }[]): MockedCurrentUser;
function setCurrentUser(providerData: null): null;
function setCurrentUser(
  providerData: { providerId: string }[] | null,
): MockedCurrentUser | null {
  const instance = mockedAuth();
  if (providerData === null) {
    instance.currentUser = null;
    return null;
  }
  const user: MockedCurrentUser = {
    providerData,
    linkWithCredential: jest.fn().mockResolvedValue(undefined),
    unlink: jest.fn().mockResolvedValue(undefined),
  };
  instance.currentUser = user;
  return user;
}

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

describe("connected accounts", () => {
  beforeEach(() => jest.clearAllMocks());

  it("linkedProviderIds maps providerData", async () => {
    setCurrentUser([{ providerId: "password" }, { providerId: "google.com" }]);
    await expect(linkedProviderIds()).resolves.toEqual(["password", "google.com"]);
  });

  it("linkedProviderIds returns [] when signed out", async () => {
    setCurrentUser(null);
    await expect(linkedProviderIds()).resolves.toEqual([]);
  });

  it("linkGoogleToCurrentUser links the built credential to currentUser", async () => {
    const user = setCurrentUser([{ providerId: "password" }]);
    await linkGoogleToCurrentUser("gtok");
    expect(mockedAuth.GoogleAuthProvider.credential).toHaveBeenCalledWith("gtok");
    expect(user.linkWithCredential).toHaveBeenCalledWith({ provider: "google", idToken: "gtok" });
  });

  it("linkAppleToCurrentUser links the built credential to currentUser", async () => {
    const user = setCurrentUser([{ providerId: "password" }]);
    await linkAppleToCurrentUser("atok", "nonce");
    expect(mockedAuth.AppleAuthProvider.credential).toHaveBeenCalledWith("atok", "nonce");
    expect(user.linkWithCredential).toHaveBeenCalledWith({
      provider: "apple",
      idToken: "atok",
      nonce: "nonce",
    });
  });

  it("unlinkProvider removes a provider when more than one remains", async () => {
    const user = setCurrentUser([{ providerId: "password" }, { providerId: "google.com" }]);
    await unlinkProvider("google.com");
    expect(user.unlink).toHaveBeenCalledWith("google.com");
  });

  it("unlinkProvider REFUSES to remove the last method and never calls unlink", async () => {
    const user = setCurrentUser([{ providerId: "password" }]);
    await expect(unlinkProvider("password")).rejects.toBeInstanceOf(LastSignInMethodError);
    expect(user.unlink).not.toHaveBeenCalled();
  });

  it("link/unlink throw when signed out", async () => {
    setCurrentUser(null);
    await expect(linkGoogleToCurrentUser("g")).rejects.toThrow(/not signed in/i);
    await expect(unlinkProvider("google.com")).rejects.toThrow(/not signed in/i);
  });
});
