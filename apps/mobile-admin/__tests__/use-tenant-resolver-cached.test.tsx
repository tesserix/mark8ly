import { renderHook } from '@testing-library/react-native';
import { ApiError } from '@repo/mobile-shared/api/client';
import { useTenantStore } from '@repo/mobile-shared/stores/tenant-store';
import { useTenantResolver } from '@/lib/hooks/use-tenant-resolver';
import { useStores } from '@/lib/hooks/use-store';
import type { Store } from '@repo/mobile-shared/api/types';

jest.mock('@/lib/hooks/use-store', () => ({
  useStores: jest.fn(),
}));

jest.mock('expo-secure-store', () => ({
  getItemAsync: jest.fn(async () => null),
  setItemAsync: jest.fn(async () => undefined),
  deleteItemAsync: jest.fn(async () => undefined),
}));

const mockUseStores = useStores as jest.Mock;

const STORE: Store = {
  id: 'store_1',
  name: 'Acme',
  slug: 'acme',
  country_code: 'IN',
  currency_code: 'INR',
  status: 'active',
};

function mockStoresQuery(overrides: Record<string, unknown> = {}) {
  mockUseStores.mockReturnValue({
    data: undefined,
    isLoading: false,
    isError: false,
    error: null,
    refetch: jest.fn(),
    ...overrides,
  });
}

beforeEach(() => {
  mockUseStores.mockReset();
  useTenantStore.setState({
    tenantId: null,
    activeStore: null,
    stores: [],
    hydrated: true,
  });
});

describe('useTenantResolver — hydrated store', () => {
  it('lets a returning user in while /stores is still in flight', () => {
    useTenantStore.setState({ activeStore: STORE, tenantId: STORE.id });
    mockStoresQuery({ isLoading: true });

    const { result } = renderHook(() => useTenantResolver());

    expect(result.current.resolving).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it('does not gate the app when the /stores refetch fails', () => {
    useTenantStore.setState({ activeStore: STORE, tenantId: STORE.id });
    mockStoresQuery({ isError: true, error: new Error('Network request failed') });

    const { result } = renderHook(() => useTenantResolver());

    expect(result.current.resolving).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it('still surfaces a 403 — a verdict, not a blip', () => {
    useTenantStore.setState({ activeStore: STORE, tenantId: STORE.id });
    const denial = new ApiError(403, 'forbidden', 'Forbidden');
    mockStoresQuery({ isError: true, error: denial });

    const { result } = renderHook(() => useTenantResolver());

    expect(result.current.error).toBe(denial);
  });

  it('surfaces a network error when there is no cached store to fall back on', () => {
    const failure = new Error('Network request failed');
    mockStoresQuery({ isError: true, error: failure });

    const { result } = renderHook(() => useTenantResolver());

    expect(result.current.error).toBe(failure);
  });

  it('reports resolving until hydration finishes, even with a cached store', () => {
    useTenantStore.setState({ activeStore: STORE, hydrated: false });
    mockStoresQuery({ isLoading: true });

    const { result } = renderHook(() => useTenantResolver());

    expect(result.current.resolving).toBe(true);
  });
});
