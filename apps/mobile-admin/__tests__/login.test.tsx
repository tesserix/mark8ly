import { fireEvent, render, waitFor } from '@testing-library/react-native';
import LoginScreen from '../app/login';
import { signInWithGoogleNative } from '@/lib/social-auth';
import { AuthCancelledError } from '@repo/mobile-shared/auth/errors';
import { useAuthNoticeStore } from '@repo/mobile-shared/stores/auth-notice';

// The monorepo hoists `zustand` (and the `react` it requires) to the repo
// root, at a different react version than this app's local install. Any
// zustand store rendered through a component in this app's tests hits two
// live React copies and crashes with "Cannot read properties of null
// (reading 'useCallback')". Swap in a minimal `create()` built on this
// test file's own `useSyncExternalStore` so `@repo/mobile-shared/stores/
// auth-notice` (unmocked, real module) works under jest without touching
// jest.config.js's moduleNameMapper.
jest.mock('zustand', () => {
  const { useSyncExternalStore } = require('react');
  function isUpdater(value: unknown): value is (current: unknown) => unknown {
    return typeof value === 'function';
  }
  function create(initializer: (set: (partial: unknown) => void, get: () => unknown) => unknown) {
    let state: unknown;
    const listeners = new Set<() => void>();
    function setState(partial: unknown): void {
      const next = isUpdater(partial) ? partial(state) : partial;
      state = { ...(state as object), ...(next as object) };
      listeners.forEach((listener) => listener());
    }
    function getState(): unknown {
      return state;
    }
    function subscribe(listener: () => void): () => void {
      listeners.add(listener);
      return () => listeners.delete(listener);
    }
    state = initializer(setState, getState);
    function useBoundStore(selector?: (current: unknown) => unknown) {
      return useSyncExternalStore(subscribe, () => (selector ? selector(getState()) : getState()));
    }
    return Object.assign(useBoundStore, { getState, setState, subscribe });
  }
  return { create };
});

const mockSignIn = jest.fn();
const mockAuth: Record<string, unknown> = {};

jest.mock('@repo/mobile-shared/auth/provider', () => ({
  useAuth: () => mockAuth,
}));

jest.mock('@/lib/social-auth', () => ({
  configureGoogleSignin: jest.fn(),
  signInWithGoogleNative: jest.fn().mockResolvedValue('gtok'),
  signInWithAppleNative: jest.fn().mockResolvedValue({ idToken: 'atok', rawNonce: '', fullName: null }),
}));

jest.mock('../components/auth/LinkAccountPrompt', () => ({
  LinkAccountPrompt: ({ email }: { email: string }) => {
    const { Text } = require('react-native');
    return <Text testID="link-prompt">{`link:${email}`}</Text>;
  },
}));

function mockUseAuth(overrides: Record<string, unknown> = {}) {
  Object.assign(
    mockAuth,
    {
      signIn: mockSignIn,
      signInWithGoogle: jest.fn().mockResolvedValue({ status: 'signed-in' }),
      signInWithApple: jest.fn().mockResolvedValue({ status: 'signed-in' }),
      loading: false,
    },
    overrides,
  );
}

beforeEach(() => {
  mockSignIn.mockReset();
  mockSignIn.mockResolvedValue(undefined);
  for (const key of Object.keys(mockAuth)) {
    delete mockAuth[key];
  }
  mockUseAuth();
});

describe('LoginScreen', () => {
  it('renders the brand wordmark and a sign-in action', () => {
    const { getByText, getByLabelText } = render(<LoginScreen />);
    expect(getByText('Mark8ly')).toBeTruthy();
    expect(getByLabelText('Sign in')).toBeTruthy();
  });

  it('invokes signIn with the entered credentials', async () => {
    const { getByLabelText } = render(<LoginScreen />);
    fireEvent.changeText(getByLabelText('Email'), 'merchant@store.com');
    fireEvent.changeText(getByLabelText('Password'), 'hunter2');
    fireEvent.press(getByLabelText('Sign in'));
    await waitFor(() =>
      expect(mockSignIn).toHaveBeenCalledWith('merchant@store.com', 'hunter2'),
    );
  });

  it('shows mapped copy — never the raw message — when signIn rejects', async () => {
    mockSignIn.mockRejectedValue(
      Object.assign(new Error('INVALID_LOGIN_CREDENTIALS'), { code: 'auth/invalid-credential' }),
    );
    const { getByLabelText, findByText, queryByText } = render(<LoginScreen />);
    fireEvent.press(getByLabelText('Sign in'));
    expect(await findByText(/check your details/i)).toBeTruthy();
    expect(queryByText('INVALID_LOGIN_CREDENTIALS')).toBeNull();
  });

  it('disables the button and shows "Signing in…" while a sign-in is in flight', async () => {
    let resolveSignIn: () => void = () => {};
    const deferred = new Promise<void>((resolve) => {
      resolveSignIn = resolve;
    });
    mockSignIn.mockReturnValue(deferred);

    const { getByLabelText, findByText, queryByText } = render(<LoginScreen />);
    const button = getByLabelText('Sign in');
    fireEvent.press(button);

    // In-flight: label flips to the busy state and the button reports disabled.
    expect(await findByText('Signing in…')).toBeTruthy();
    expect(button.props.accessibilityState?.disabled).toBe(true);

    resolveSignIn();

    // Settled: back to the idle label.
    await waitFor(() => expect(queryByText('Signing in…')).toBeNull());
  });

  it('signs in with Google when the Google button is pressed', async () => {
    const signInWithGoogle = jest.fn().mockResolvedValue({ status: 'signed-in' });
    mockUseAuth({ signInWithGoogle });
    const { getByLabelText } = render(<LoginScreen />);
    fireEvent.press(getByLabelText('Continue with Google'));
    await waitFor(() => expect(signInWithGoogle).toHaveBeenCalledWith('gtok'));
  });

  it('signs in with Apple when the Apple button is pressed', async () => {
    const signInWithApple = jest.fn().mockResolvedValue({ status: 'signed-in' });
    mockUseAuth({ signInWithApple });
    const { getByLabelText } = render(<LoginScreen />);
    fireEvent.press(getByLabelText('Sign in with Apple'));
    await waitFor(() => expect(signInWithApple).toHaveBeenCalledWith('atok', '', null));
  });

  it('opens the link prompt when Google sign-in needs linking', async () => {
    const signInWithGoogle = jest.fn().mockResolvedValue({
      status: 'needs-link',
      email: 'merchant@store.com',
      provider: 'google.com',
      pendingCredential: { provider: 'google', idToken: 'gtok' },
    });
    mockUseAuth({ signInWithGoogle });
    const { getByLabelText, findByTestId } = render(<LoginScreen />);
    fireEvent.press(getByLabelText('Continue with Google'));
    expect(await findByTestId('link-prompt')).toBeTruthy();
  });

  it('does not open the link prompt on a normal signed-in outcome', async () => {
    const signInWithGoogle = jest.fn().mockResolvedValue({ status: 'signed-in' });
    mockUseAuth({ signInWithGoogle });
    const { getByLabelText, queryByTestId } = render(<LoginScreen />);
    fireEvent.press(getByLabelText('Continue with Google'));
    await waitFor(() => expect(signInWithGoogle).toHaveBeenCalled());
    expect(queryByTestId('link-prompt')).toBeNull();
  });
});

describe('error copy', () => {
  it('shows NOTHING when the user cancels the Google sheet', async () => {
    (signInWithGoogleNative as jest.Mock).mockRejectedValueOnce(new AuthCancelledError());
    const { getByLabelText, queryByText } = render(<LoginScreen />);
    fireEvent.press(getByLabelText('Continue with Google'));
    await waitFor(() => expect(signInWithGoogleNative).toHaveBeenCalled());
    expect(queryByText(/cancel/i)).toBeNull();
    expect(queryByText(/something went wrong/i)).toBeNull();
  });

  it('never shows a raw native error string', async () => {
    (signInWithGoogleNative as jest.Mock).mockRejectedValueOnce(
      new Error('RequestUnknownException: AppleAuthenticationExceptions.swift:61'),
    );
    const { getByLabelText, findByText, queryByText } = render(<LoginScreen />);
    fireEvent.press(getByLabelText('Continue with Google'));
    expect(await findByText('Something went wrong. Try again.')).toBeTruthy();
    expect(queryByText(/swift/i)).toBeNull();
  });
});

describe('involuntary sign-out notice', () => {
  afterEach(() => useAuthNoticeStore.getState().clearNotice());

  it('explains an access-denied sign-out', async () => {
    useAuthNoticeStore.getState().setNotice('access-denied');
    const { findByText } = render(<LoginScreen />);
    expect(await findByText(/doesn't have access to a Mark8ly admin account/i)).toBeTruthy();
  });

  it('explains a dead session', async () => {
    useAuthNoticeStore.getState().setNotice('no-session');
    const { findByText } = render(<LoginScreen />);
    expect(await findByText(/your session ended/i)).toBeTruthy();
  });

  it('clears the notice so it cannot resurface on a later attempt', async () => {
    useAuthNoticeStore.getState().setNotice('access-denied');
    const { findByText } = render(<LoginScreen />);
    await findByText(/doesn't have access/i);
    await waitFor(() => expect(useAuthNoticeStore.getState().notice).toBeNull());
  });

  it('shows nothing when there is no notice', () => {
    const { queryByText } = render(<LoginScreen />);
    expect(queryByText(/session ended|doesn't have access/i)).toBeNull();
  });
});
