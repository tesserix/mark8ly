import { fireEvent, render, waitFor } from '@testing-library/react-native';
import LoginScreen from '../app/login';

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

  it('shows the error message when signIn rejects', async () => {
    mockSignIn.mockRejectedValue(new Error('Wrong password'));
    const { getByLabelText, findByText } = render(<LoginScreen />);
    fireEvent.press(getByLabelText('Sign in'));
    expect(await findByText('Wrong password')).toBeTruthy();
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
