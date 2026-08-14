import { useTenantStore } from '@repo/mobile-shared/stores/tenant-store';
import type { Store } from '@repo/mobile-shared/api/types';

// In-memory stand-in for the keychain. expo-secure-store has no native module
// under jest, and these tests are about what survives a cold launch — so the
// backing map persists across `useTenantStore` resets the way the keychain
// persists across process restarts.
jest.mock('expo-secure-store', () => {
  const items = new Map<string, string>();
  return {
    __items: items,
    getItemAsync: jest.fn(async (k: string) => items.get(k) ?? null),
    setItemAsync: jest.fn(async (k: string, v: string) => {
      items.set(k, v);
    }),
    deleteItemAsync: jest.fn(async (k: string) => {
      items.delete(k);
    }),
  };
});

const secureStore = require('expo-secure-store') as {
  __items: Map<string, string>;
};

const STORE: Store = {
  id: 'store_1',
  name: 'Acme',
  slug: 'acme',
  country_code: 'IN',
  currency_code: 'INR',
  status: 'active',
};

/** Wipes in-memory state the way a cold launch does, keeping the keychain. */
function relaunch() {
  useTenantStore.setState({
    tenantId: null,
    activeStore: null,
    stores: [],
    hydrated: false,
  });
}

beforeEach(() => {
  secureStore.__items.clear();
  relaunch();
});

describe('tenant-store hydration', () => {
  it('restores the full active store after a cold launch', async () => {
    useTenantStore.getState().setActiveStore(STORE);
    await new Promise<void>((resolve) => {
      setImmediate(() => resolve());
    });

    relaunch();
    await useTenantStore.getState().hydrate();

    expect(useTenantStore.getState().activeStore).toEqual(STORE);
    expect(useTenantStore.getState().tenantId).toBe(STORE.id);
  });

  it('discards a persisted store that no longer matches the schema', async () => {
    useTenantStore.getState().setActiveStore(STORE);
    await new Promise<void>((resolve) => {
      setImmediate(() => resolve());
    });
    secureStore.__items.set(
      'mark8ly_active_store',
      JSON.stringify({ id: 'store_1', name: 'Acme' }),
    );

    relaunch();
    await useTenantStore.getState().hydrate();

    expect(useTenantStore.getState().activeStore).toBeNull();
    expect(useTenantStore.getState().hydrated).toBe(true);
  });

  it('discards a persisted store whose id no longer matches the remembered one', async () => {
    useTenantStore.getState().setActiveStore(STORE);
    await new Promise<void>((resolve) => {
      setImmediate(() => resolve());
    });
    secureStore.__items.set('mark8ly_store_id', 'store_2');

    relaunch();
    await useTenantStore.getState().hydrate();

    expect(useTenantStore.getState().activeStore).toBeNull();
  });

  it('hydrates to no store when nothing was ever persisted', async () => {
    await useTenantStore.getState().hydrate();

    expect(useTenantStore.getState().activeStore).toBeNull();
    expect(useTenantStore.getState().tenantId).toBeNull();
    expect(useTenantStore.getState().hydrated).toBe(true);
  });

  it('forgets the persisted store when the active store is cleared', async () => {
    useTenantStore.getState().setActiveStore(STORE);
    await new Promise<void>((resolve) => {
      setImmediate(() => resolve());
    });

    await useTenantStore.getState().clearActiveStore();
    relaunch();
    await useTenantStore.getState().hydrate();

    expect(useTenantStore.getState().activeStore).toBeNull();
  });
});
