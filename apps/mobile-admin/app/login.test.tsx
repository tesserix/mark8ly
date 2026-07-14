import { render } from '@testing-library/react-native';
import LoginScreen from './login';

jest.mock('@repo/mobile-shared/auth/provider', () => ({
  useAuth: () => ({ signIn: jest.fn(), loading: false }),
}));

describe('LoginScreen', () => {
  it('renders the brand wordmark and a sign-in action', () => {
    const { getByText, getByLabelText } = render(<LoginScreen />);
    expect(getByText('Mark8ly')).toBeTruthy();
    expect(getByLabelText('Sign in')).toBeTruthy();
  });
});
