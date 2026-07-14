import { fireEvent, render, waitFor } from '@testing-library/react-native';
import LoginScreen from './login';

const mockSignIn = jest.fn();

jest.mock('@repo/mobile-shared/auth/provider', () => ({
  useAuth: () => ({ signIn: mockSignIn, loading: false }),
}));

beforeEach(() => {
  mockSignIn.mockReset();
  mockSignIn.mockResolvedValue(undefined);
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
});
