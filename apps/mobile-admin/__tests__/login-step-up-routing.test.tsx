// Where the login screen sends an outstanding step-up (#686 item 2).
//
// A TOTP challenge must reach /totp, not /otp. Routing it to the emailed
// screen sends a merchant to their inbox for a code that only ever exists
// inside an authenticator app — and before this branch it did not even get
// that far: the challenge carried nothing to resume with, so the screen
// showed "this app version needs an update", which no update could fix.
import { fireEvent, render, waitFor } from '@testing-library/react-native';
import LoginScreen from '../app/login';

const mockPush = jest.fn();
const mockReplace = jest.fn();
const mockSignIn = jest.fn();

jest.mock('expo-router', () => ({
  router: {
    push: (...args: unknown[]) => mockPush(...args),
    replace: (...args: unknown[]) => mockReplace(...args),
  },
}));

jest.mock('@repo/mobile-shared/auth/zitadel-signin', () => ({
  createZitadelSignIn: () => ({ signIn: mockSignIn }),
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
  signInWithAppleNative: jest.fn(),
}));

jest.mock('../components/auth/LinkAccountPrompt', () => ({
  LinkAccountPrompt: () => null,
}));

beforeEach(() => {
  mockPush.mockReset();
  mockReplace.mockReset();
  mockSignIn.mockReset();
});

function submit() {
  const view = render(<LoginScreen />);
  fireEvent.changeText(view.getByLabelText('Email'), 'a@b.test');
  fireEvent.changeText(view.getByLabelText('Password'), 'pw');
  fireEvent.press(view.getByLabelText('Sign in'));
  return view;
}

it('sends a TOTP step-up to the authenticator screen, not /otp', async () => {
  mockSignIn.mockResolvedValue({ kind: 'totp', email: 'a@b.test', tenantId: 't-1', pendingToken: 'sealed' });

  submit();

  await waitFor(() =>
    expect(mockPush).toHaveBeenCalledWith({
      pathname: '/totp',
      params: { pendingToken: 'sealed' },
    }),
  );
});

it('still sends an emailed step-up to /otp, with the address to show', async () => {
  mockSignIn.mockResolvedValue({ kind: 'otp', email: 'a@b.test', tenantId: 't-1', pendingToken: 'sealed' });

  submit();

  await waitFor(() =>
    expect(mockPush).toHaveBeenCalledWith({
      pathname: '/otp',
      params: { pendingToken: 'sealed', email: 'a@b.test' },
    }),
  );
});

it('goes straight to the app when nothing is outstanding', async () => {
  mockSignIn.mockResolvedValue({ kind: 'signed_in', email: 'a@b.test', tenantId: 't-1' });

  submit();

  await waitFor(() => expect(mockReplace).toHaveBeenCalledWith('/(tabs)'));
  expect(mockPush).not.toHaveBeenCalled();
});

// The "needs an update" copy stays for its real case — a challenge the
// server genuinely could not seal — but a TOTP enrolment must never reach
// it again.
it('does not show the update-required copy for a TOTP step-up', async () => {
  mockSignIn.mockResolvedValue({ kind: 'totp', email: 'a@b.test', tenantId: 't-1', pendingToken: 'sealed' });

  const view = submit();

  await waitFor(() => expect(mockPush).toHaveBeenCalled());
  expect(view.queryByText(/needs an update/i)).toBeNull();
});
