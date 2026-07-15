const mockAuth: Record<string, unknown> = {};
jest.mock("@repo/mobile-shared/auth/provider", () => ({
  useAuth: () => mockAuth,
}));
jest.mock("@/lib/social-auth", () => ({
  configureGoogleSignin: jest.fn(),
  signInWithGoogleNative: jest.fn().mockResolvedValue("existing-gtok"),
  signInWithAppleNative: jest
    .fn()
    .mockResolvedValue({ idToken: "existing-atok", rawNonce: "", fullName: null }),
}));

import { fireEvent, render, waitFor } from "@testing-library/react-native";
import { LinkAccountPrompt } from "../components/auth/LinkAccountPrompt";

const PENDING = { provider: "google", idToken: "pending-tok" } as never;

function setAuth(overrides: Record<string, unknown> = {}) {
  Object.keys(mockAuth).forEach((k) => delete mockAuth[k]);
  Object.assign(
    mockAuth,
    {
      existingSignInMethods: jest.fn().mockResolvedValue(["password"]),
      completeLinkWithPassword: jest.fn().mockResolvedValue(undefined),
      completeLinkWithGoogle: jest.fn().mockResolvedValue(undefined),
      completeLinkWithApple: jest.fn().mockResolvedValue(undefined),
    },
    overrides,
  );
}

// `provider` is the provider being LINKED (the one that hit the conflict).
function renderPrompt(
  provider: 'google.com' | 'apple.com' = 'google.com',
  onLinked = jest.fn(),
  onCancel = jest.fn(),
) {
  return {
    onLinked,
    onCancel,
    ...render(
      <LinkAccountPrompt
        visible
        email="merchant@store.com"
        provider={provider}
        pendingCredential={PENDING}
        onCancel={onCancel}
        onLinked={onLinked}
      />,
    ),
  };
}

describe("LinkAccountPrompt", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    setAuth();
  });

  it("shows the conflicting email and the provider being linked", async () => {
    const { getByText } = renderPrompt();
    await waitFor(() => expect(getByText(/merchant@store.com/)).toBeTruthy());
    expect(getByText(/Google/)).toBeTruthy();
  });

  // Linking Google onto an existing password account (the common case).
  it("password method: submitting links with the password", async () => {
    const { getByLabelText, onLinked } = renderPrompt("google.com");
    await waitFor(() => expect(getByLabelText("Password")).toBeTruthy());
    fireEvent.changeText(getByLabelText("Password"), "hunter2");
    fireEvent.press(getByLabelText("Sign in and link"));
    await waitFor(() =>
      expect(mockAuth.completeLinkWithPassword).toHaveBeenCalledWith(
        "merchant@store.com",
        "hunter2",
        PENDING,
      ),
    );
    await waitFor(() => expect(onLinked).toHaveBeenCalled());
  });

  // Linking APPLE onto an account whose existing method is Google — the only
  // coherent way a Google re-auth button appears (you can never re-auth with
  // the same provider you are linking).
  it("google method: offers a Google re-auth button that links", async () => {
    setAuth({ existingSignInMethods: jest.fn().mockResolvedValue(["google.com"]) });
    const { getByLabelText, onLinked } = renderPrompt("apple.com");
    await waitFor(() => expect(getByLabelText("Continue with Google to link")).toBeTruthy());
    fireEvent.press(getByLabelText("Continue with Google to link"));
    await waitFor(() =>
      expect(mockAuth.completeLinkWithGoogle).toHaveBeenCalledWith("existing-gtok", PENDING),
    );
    await waitFor(() => expect(onLinked).toHaveBeenCalled());
  });

  // Enumeration protection hides the answer: offer password + every OTHER
  // provider, but never the provider being linked.
  it("enumeration-protected ([]): shows password plus the other providers, not the one being linked", async () => {
    setAuth({ existingSignInMethods: jest.fn().mockResolvedValue([]) });
    const { getByLabelText, queryByLabelText } = renderPrompt("google.com");
    await waitFor(() => expect(getByLabelText("Password")).toBeTruthy());
    expect(getByLabelText("Continue with Apple to link")).toBeTruthy();
    expect(queryByLabelText("Continue with Google to link")).toBeNull();
  });

  it("shows an error and stays open when the re-auth fails", async () => {
    setAuth({
      completeLinkWithPassword: jest.fn().mockRejectedValue(new Error("Wrong password")),
    });
    const { getByLabelText, findByText, onLinked } = renderPrompt();
    await waitFor(() => expect(getByLabelText("Password")).toBeTruthy());
    fireEvent.changeText(getByLabelText("Password"), "nope");
    fireEvent.press(getByLabelText("Sign in and link"));
    expect(await findByText("Wrong password")).toBeTruthy();
    expect(onLinked).not.toHaveBeenCalled();
  });

  it("cancel closes without linking", async () => {
    const { getByLabelText, onCancel } = renderPrompt();
    await waitFor(() => expect(getByLabelText("Password")).toBeTruthy());
    fireEvent.press(getByLabelText("Cancel"));
    expect(onCancel).toHaveBeenCalled();
    expect(mockAuth.completeLinkWithPassword).not.toHaveBeenCalled();
  });
});
