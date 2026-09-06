/**
 * The Apple button on a Zitadel build (#771).
 *
 * Before this, handleAppleSignIn had no Zitadel branch: it authenticated
 * against Firebase and set the provider's `user`, which AuthGate ignores
 * under Zitadel (it reads zitadelSignedIn) and api-client ignores too (it
 * reads zitadelSession) — a silent bounce back to /login. The button was
 * hidden for exactly that reason; these tests are what let it come back.
 */
import { fireEvent, render, waitFor } from '@testing-library/react-native';
import LoginScreen from '../app/login';
import { ZitadelAuthError } from '@repo/mobile-shared/auth/zitadel-client';

const mockPush = jest.fn();
const mockReplace = jest.fn();
const mockSignInWithApple = jest.fn();
const mockNativeApple = jest.fn();

jest.mock('expo-router', () => ({
  router: {
    push: (...args: unknown[]) => mockPush(...args),
    replace: (...args: unknown[]) => mockReplace(...args),
  },
}));

jest.mock('@repo/mobile-shared/auth/zitadel-signin', () => ({
  createZitadelSignIn: () => ({ signInWithApple: mockSignInWithApple }),
}));

jest.mock('@repo/mobile-shared/auth/provider', () => ({
  useAuth: () => ({ signIn: jest.fn(), signInWithGoogle: jest.fn(), signInWithApple: jest.fn() }),
}));

jest.mock('@repo/mobile-shared/config/env', () => ({
  useEnvironment: () => ({ apiBaseUrl: 'https://api.mark8ly.test' }),
}));

jest.mock('@repo/mobile-shared/stores/tenant-store', () => ({
  useTenantStore: (selector: (s: unknown) => unknown) => selector({ setTenantId: jest.fn() }),
}));

jest.mock('@/lib/auth-provider', () => ({ isZitadelProvider: () => true }));

jest.mock('@/lib/social-auth', () => ({
  configureGoogleSignin: jest.fn(),
  signInWithGoogleNative: jest.fn(),
  signInWithAppleNative: (...args: unknown[]) => mockNativeApple(...args),
}));

jest.mock('../components/auth/LinkAccountPrompt', () => ({
  LinkAccountPrompt: () => null,
}));

beforeEach(() => {
  mockPush.mockReset();
  mockReplace.mockReset();
  mockSignInWithApple.mockReset();
  mockNativeApple.mockReset();
});

function pressApple() {
  const view = render(<LoginScreen />);
  fireEvent.press(view.getByTestId('provider-apple'));
  return view;
}

it('signs in through Zitadel, never the native Apple sheet', async () => {
  mockSignInWithApple.mockResolvedValue({ kind: 'signed_in', email: 'a@b.test', tenantId: 't-1' });

  pressApple();

  await waitFor(() => expect(mockReplace).toHaveBeenCalledWith('/(tabs)'));
  // The Firebase path is what left the app half-signed-in; it must not run.
  expect(mockNativeApple).not.toHaveBeenCalled();
  expect(mockSignInWithApple).toHaveBeenCalledWith(
    expect.objectContaining({ redirectUrl: 'mark8ly-admin://auth/idp' }),
    expect.any(Function),
  );
});

it('routes an emailed step-up to /otp, like the Google path', async () => {
  mockSignInWithApple.mockResolvedValue({
    kind: 'otp',
    email: 'a@b.test',
    tenantId: 't-1',
    pendingToken: 'sealed',
  });

  pressApple();

  await waitFor(() =>
    expect(mockPush).toHaveBeenCalledWith({
      pathname: '/otp',
      params: { pendingToken: 'sealed', email: 'a@b.test' },
    }),
  );
  expect(mockReplace).not.toHaveBeenCalled();
});

it('routes an authenticator step-up to /totp', async () => {
  mockSignInWithApple.mockResolvedValue({
    kind: 'totp',
    email: 'a@b.test',
    tenantId: 't-1',
    pendingToken: 'sealed',
  });

  pressApple();

  await waitFor(() =>
    expect(mockPush).toHaveBeenCalledWith({
      pathname: '/totp',
      params: { pendingToken: 'sealed' },
    }),
  );
});

// The copy is the point: this screen renders its own mapping of the error
// CODE and never the server's message, so an Apple failure shown as a Google
// one tells the merchant the wrong button broke.
it('names Apple, not Google, when the Apple sign-in fails', async () => {
  mockSignInWithApple.mockRejectedValue(
    new ZitadelAuthError('apple_sign_in_failed', 'server copy that is never rendered'),
  );

  const view = pressApple();

  const msg = await view.findByText(/Couldn't sign you in with Apple/i);
  expect(msg).toBeTruthy();
  expect(view.queryByText(/with Google/i)).toBeNull();
});

it('names Apple in the unverified-email refusal too', async () => {
  mockSignInWithApple.mockRejectedValue(new ZitadelAuthError('email_not_verified', ''));

  const view = pressApple();

  await view.findByText(/Apple hasn't verified that email address/i);
  expect(view.queryByText(/Google hasn't verified/i)).toBeNull();
});

// Dismissing the browser sheet is a decision, not a failure.
it('stays silent when the merchant cancels', async () => {
  mockSignInWithApple.mockRejectedValue(new ZitadelAuthError('cancelled', ''));

  const view = pressApple();

  await waitFor(() => expect(mockSignInWithApple).toHaveBeenCalled());
  expect(view.queryByRole('alert')).toBeNull();
});
