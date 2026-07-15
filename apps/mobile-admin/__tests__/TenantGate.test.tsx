import { fireEvent, render } from '@testing-library/react-native';
import { ApiError } from '@repo/mobile-shared/api/client';
import { TenantGate } from '@/components/TenantGate';
import { useTenantResolver } from '@/lib/hooks/use-tenant-resolver';

// Mocks are built INSIDE each jest.mock factory (babel hoists imports above
// outer const/var) and read back off the imported module below.
jest.mock('@/lib/hooks/use-tenant-resolver', () => ({
  useTenantResolver: jest.fn(),
}));

jest.mock('@tanstack/react-query', () => ({
  useQueryClient: jest.fn(() => ({ invalidateQueries: jest.fn() })),
}));

// StorePicker pulls in lucide-react-native (ESM, untransformed under jest)
// purely for an icon it doesn't even need in the error-path tests below.
// Stub it out rather than widening transformIgnorePatterns.
jest.mock('@/components/StorePicker', () => ({
  StorePicker: () => null,
}));

// `@/components/ui` barrel re-exports BackHeader/SearchField, which import
// icons from lucide-react-native's ESM build (`dist/esm/...mjs`) — not
// covered by jest-expo's default transformIgnorePatterns, so requiring it
// unmocked throws "Unexpected token 'export'". Stub every icon export with a
// no-op component (same fix as __tests__/security.test.tsx).
jest.mock('lucide-react-native', () => {
  const IconStub = () => null;
  return new Proxy({}, { get: () => IconStub });
});
// `@/components/ui`'s `Screen` calls `useSafeAreaInsets()`, which throws
// without a `<SafeAreaProvider>` ancestor. react-native-safe-area-context
// ships an official jest mock for exactly this (same fix as
// __tests__/security.test.tsx).
jest.mock('react-native-safe-area-context', () => {
  const mock = require('react-native-safe-area-context/jest/mock');
  return { __esModule: true, ...mock.default };
});

const mockUseTenantResolver = useTenantResolver as jest.Mock;

function mockResolverState(overrides: Record<string, unknown> = {}) {
  mockUseTenantResolver.mockReturnValue({
    resolving: false,
    needsOnboarding: false,
    needsPicker: false,
    availableStores: [],
    error: null,
    refetch: jest.fn(),
    ...overrides,
  });
}

beforeEach(() => {
  mockUseTenantResolver.mockReset();
});

describe('TenantGate — error states', () => {
  it('shows the access-denied copy with no Retry button on a 403', () => {
    const refetch = jest.fn();
    mockResolverState({ error: new ApiError(403, 'forbidden', 'Forbidden'), refetch });

    const { getByText, queryByLabelText } = render(
      <TenantGate>
        <></>
      </TenantGate>,
    );

    expect(getByText('No access')).toBeTruthy();
    expect(
      getByText(/doesn't have access to a Mark8ly admin account/i),
    ).toBeTruthy();
    expect(queryByLabelText('Retry')).toBeNull();
  });

  it('shows the connection-error copy with a working Retry button on a non-403 error', () => {
    const refetch = jest.fn();
    mockResolverState({ error: new Error('Network request failed'), refetch });

    const { getByText, getByLabelText } = render(
      <TenantGate>
        <></>
      </TenantGate>,
    );

    expect(getByText("Couldn't load your store")).toBeTruthy();
    expect(getByText(/check your connection/i)).toBeTruthy();

    const retryButton = getByLabelText('Retry');
    expect(retryButton).toBeTruthy();
    fireEvent.press(retryButton);
    expect(refetch).toHaveBeenCalledTimes(1);
  });

  it('treats a non-403 ApiError the same as a generic error — Retry offered', () => {
    const refetch = jest.fn();
    mockResolverState({ error: new ApiError(500, 'server_error', 'Server error'), refetch });

    const { getByText, getByLabelText } = render(
      <TenantGate>
        <></>
      </TenantGate>,
    );

    expect(getByText("Couldn't load your store")).toBeTruthy();
    expect(getByLabelText('Retry')).toBeTruthy();
  });
});
