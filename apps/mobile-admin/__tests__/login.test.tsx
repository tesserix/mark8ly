import { configure, fireEvent, render, waitFor, within } from '@testing-library/react-native';
import { StyleSheet } from 'react-native';
import { Path } from 'react-native-svg';
import LoginScreen from '../app/login';
import { theme } from '@/lib/theme';

// LoginScreen's in-flight → idle re-render takes ~700ms even in isolation on a
// fast machine (jest-expo render + the auth/query stack), leaving almost no
// headroom under RTL's default 1000ms async timeout. On a loaded CI runner the
// re-render crosses 1000ms and the "Signing in…" waitFor times out — a
// deterministic CI failure that passes locally. Raise the ceiling for this
// file; the assertions still fail if the condition never holds, they just get
// enough time on a slow runner.
configure({ asyncUtilTimeout: 5000 });
import { signInWithGoogleNative } from '@/lib/social-auth';
import { AuthCancelledError } from '@repo/mobile-shared/auth/errors';
import { useAuthNoticeStore } from '@repo/mobile-shared/stores/auth-notice';

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
    fireEvent.press(getByLabelText('Sign in'));

    // In-flight: label flips to the busy state and the button reports disabled.
    expect(await findByText('Signing in…')).toBeTruthy();
    // Re-query rather than reusing an instance captured before the press: the
    // press triggers a re-render, so a reference held from before it can read
    // stale props. That raced on CI (green locally, red on a slower runner)
    // while asserting the very state the re-render produces. The
    // accessibilityLabel stays "Sign in" in both states (app/login.tsx:142).
    expect(getByLabelText('Sign in').props.accessibilityState?.disabled).toBe(true);

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

// The two providers collapsed from stacked full-width buttons into a centred
// icon row so the ink "Sign in" bar is the only full-width solid on the
// screen and reads as the unambiguous primary. Both stores treat this as a
// review-sensitive surface, so these assertions pin the brand-compliance
// requirements rather than the visual arrangement alone.
describe('provider icon row', () => {
  // react-native-svg's `extractProps` rewrites colour strings into a
  // processed `{payload,type}` object on the underlying host node and drops
  // `d` there entirely, so query by component TYPE (the composite fiber
  // still holds the original values) — same reasoning as
  // revenue-chart.test.tsx.
  function pathsIn(utils: ReturnType<typeof render>, markTestID: string) {
    return within(utils.getByTestId(markTestID)).UNSAFE_getAllByType(Path);
  }

  it('keeps the full accessible label on both providers even though the visible text is gone', () => {
    const { getByLabelText, queryByText } = render(<LoginScreen />);
    expect(getByLabelText('Continue with Google')).toBeTruthy();
    expect(getByLabelText('Sign in with Apple')).toBeTruthy();
    // Icon-only: the words must NOT be painted anywhere.
    expect(queryByText('Continue with Google')).toBeNull();
    expect(queryByText('Sign in with Apple')).toBeNull();
    // …while the primary keeps its visible text label.
    expect(queryByText('Sign in')).toBeTruthy();
  });

  it('gives each provider a real 44pt minimum box rather than a hitSlop overlay', () => {
    const { getByTestId } = render(<LoginScreen />);
    for (const testID of ['provider-google', 'provider-apple']) {
      const node = getByTestId(testID);
      const style = StyleSheet.flatten(node.props.style);
      expect(style.minWidth).toBeGreaterThanOrEqual(theme.touchTarget);
      expect(style.minHeight).toBeGreaterThanOrEqual(theme.touchTarget);
      expect(style.width).toBeGreaterThanOrEqual(theme.touchTarget);
      expect(style.height).toBeGreaterThanOrEqual(theme.touchTarget);
      expect(node.props.hitSlop).toBeUndefined();
    }
  });

  // Apple requires a logo-only Sign in with Apple button to be no LESS
  // prominent than the other providers on offer. Equal target geometry is
  // how this row satisfies that, so a future tweak that shrinks Apple must
  // fail here.
  it('renders the two provider targets at identical size', () => {
    const { getByTestId } = render(<LoginScreen />);
    const google = StyleSheet.flatten(getByTestId('provider-google').props.style);
    const apple = StyleSheet.flatten(getByTestId('provider-apple').props.style);
    expect(apple.width).toBe(google.width);
    expect(apple.height).toBe(google.height);
  });

  // The first device render had the two boxes FLUSH: the row's separation
  // was written as `className="… gap-4"`, a utility used nowhere else in
  // this app, so Tailwind's JIT never emitted it — and RNTL renders without
  // the NativeWind runtime, so every test stayed green. The row's layout is
  // now a StyleSheet value, which cannot be dropped.
  it('separates the two providers with a real resolved gap, not a class', () => {
    const { getByTestId } = render(<LoginScreen />);
    const row = StyleSheet.flatten(getByTestId('provider-row').props.style);
    expect(row.gap).toBe(theme.spacing.lg);
    expect(row.flexDirection).toBe('row');
    expect(row.justifyContent).toBe('center');
  });

  // Guards the NativeWind 4.2.5 JSX-interop landmine: a FUNCTION style prop
  // is not resolved like a plain array and the styles are silently dropped
  // at runtime with every test still green.
  it('passes a plain array style to each provider, never a function', () => {
    const { getByTestId } = render(<LoginScreen />);
    expect(typeof getByTestId('provider-google').props.style).not.toBe('function');
    expect(typeof getByTestId('provider-apple').props.style).not.toBe('function');
  });

  // An inlined SVG path that fails to render produces NOTHING on device and
  // no visual test catches it. These assert the marks are actually present
  // and carry their official geometry and colours.
  it("draws Google's official four-colour G, not a recoloured or generic glyph", () => {
    const utils = render(<LoginScreen />);
    const paths = pathsIn(utils, 'google-mark');
    expect(paths).toHaveLength(4);
    for (const path of paths) {
      expect(typeof path.props.d).toBe('string');
      expect((path.props.d as string).length).toBeGreaterThan(0);
    }
    // Google's four brand colours, verbatim — the mark must not be
    // recoloured into the Paper/Ink/Moss palette.
    expect(paths.map((p) => p.props.fill).sort()).toEqual(
      ['#4285F4', '#34A853', '#FBBC05', '#EA4335'].sort(),
    );
  });

  it("draws Apple's monochrome mark on sufficient contrast", () => {
    const utils = render(<LoginScreen />);
    const paths = pathsIn(utils, 'apple-mark');
    expect(paths).toHaveLength(1);
    const path = paths[0];
    expect(typeof path?.props.d).toBe('string');
    expect((path?.props.d as string).length).toBeGreaterThan(0);
    // Monochrome: the INK mark sits on our elevated Paper surface — the
    // official white-with-outline Sign in with Apple appearance. 17.4:1.
    expect(path?.props.fill).toBe(theme.colors.text);
    const box = StyleSheet.flatten(utils.getByTestId('provider-apple').props.style);
    expect(box.backgroundColor).toBe(theme.colors.elevated);
  });

  // The row previously paired an outlined white Google box with a SOLID INK
  // Apple box. Both were individually compliant, but the mismatch was the
  // loudest thing on the screen, and the solid fill contradicted this row's
  // own stated intent — that collapsing the providers to icons leaves the
  // full-width ink "Sign in" bar as the ONLY solid on the page.
  //
  // Pinning the two surfaces as identical is what stops that regressing:
  // asserting Apple's colours alone would still pass if Google's box were
  // the one that changed.
  it('gives both provider targets one identical surface treatment', () => {
    const { getByTestId } = render(<LoginScreen />);
    const google = StyleSheet.flatten(getByTestId('provider-google').props.style);
    const apple = StyleSheet.flatten(getByTestId('provider-apple').props.style);

    expect(apple.backgroundColor).toBe(google.backgroundColor);
    expect(apple.borderWidth).toBe(google.borderWidth);
    expect(apple.borderColor).toBe(google.borderColor);
    expect(apple.borderRadius).toBe(google.borderRadius);

    // …and that shared surface is Paper with a hairline, not a solid fill.
    expect(apple.backgroundColor).toBe(theme.colors.elevated);
    expect(apple.borderWidth).toBeGreaterThan(0);
  });

  it('disables both providers while a sign-in is in flight', async () => {
    let resolveSignIn: () => void = () => {};
    mockSignIn.mockReturnValue(new Promise<void>((resolve) => {
      resolveSignIn = resolve;
    }));
    const { getByLabelText, getByTestId } = render(<LoginScreen />);
    fireEvent.press(getByLabelText('Sign in'));
    await waitFor(() =>
      expect(getByTestId('provider-google').props.accessibilityState?.disabled).toBe(true),
    );
    expect(getByTestId('provider-apple').props.accessibilityState?.disabled).toBe(true);

    resolveSignIn();
    // Settle the in-flight → idle re-render inside the test, otherwise the
    // final setSubmitting(false) lands after teardown as an un-acted update.
    await waitFor(() =>
      expect(getByTestId('provider-google').props.accessibilityState?.disabled).toBe(false),
    );
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

// Apple must not be offered on a Zitadel build until it has a Zitadel path.
//
// handleAppleSignIn does not branch on the provider: it authenticates against
// Firebase and sets the provider's `user`, which AuthGate ignores under
// Zitadel (it reads zitadelSignedIn) and api-client ignores too (it reads
// zitadelSession). The result is a silent bounce back to /login — #493's
// shape. A button that cannot work must not be shown.
//
// This is a submission blocker, not a preference: Apple guideline 4.8 requires
// Sign in with Apple wherever another social provider is offered, and Google
// is offered on this screen. Apple has to be migrated before an App Store
// release, and this test should be deleted then, not weakened.
describe('Apple button visibility by provider', () => {
  afterEach(() => {
    delete process.env.EXPO_PUBLIC_AUTH_PROVIDER;
  });

  it('is hidden on a Zitadel build', () => {
    process.env.EXPO_PUBLIC_AUTH_PROVIDER = 'zitadel';
    const { queryByTestId } = render(<LoginScreen />);
    expect(queryByTestId('provider-apple')).toBeNull();
    // Google still is offered — it has a Zitadel path (#686 item 1).
    expect(queryByTestId('provider-google')).not.toBeNull();
  });

  it('is still shown on a GIP build, where it works', () => {
    delete process.env.EXPO_PUBLIC_AUTH_PROVIDER;
    const { queryByTestId } = render(<LoginScreen />);
    expect(queryByTestId('provider-apple')).not.toBeNull();
  });
});
