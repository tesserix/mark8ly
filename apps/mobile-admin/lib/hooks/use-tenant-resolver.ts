import { useEffect, useState } from "react";
import { useTenantStore } from "@repo/mobile-shared/stores/tenant-store";
import { tokenStorage } from "@repo/mobile-shared/auth/token-storage";
import type { Store } from "@repo/mobile-shared/api/types";
import { useStores } from "./use-store";

interface ResolverState {
  /** Still figuring out which tenant to land on. */
  resolving: boolean;
  /** User has zero stores — route them to mark8ly.com to create one. */
  needsOnboarding: boolean;
  /**
   * User has 2+ stores and no remembered selection. The UI must render a
   * picker so the user can choose which store becomes the default for
   * this device. This mirrors the admin web's /pick-tenant page.
   */
  needsPicker: boolean;
  /** Stores the picker should offer. Empty when no picker is needed. */
  availableStores: Store[];
  error: unknown;
  refetch: () => void;
}

/**
 * After sign-in, decide which store the user lands on, mirroring the
 * admin web flow:
 *
 *   • 0 stores         → onboarding empty state (sign up at mark8ly.com).
 *   • 1 store          → auto-select it. Single-tenant fast path.
 *   • 2+ stores, returning user with a still-valid last selection
 *                      → auto-restore that selection.
 *   • 2+ stores, no last selection (or it was revoked)
 *                      → expose `needsPicker` so the gate renders a
 *                        chooser. Whichever the user picks becomes the
 *                        new default; later they can change it from the
 *                        in-app store switcher.
 */
export function useTenantResolver(): ResolverState {
  const activeStore = useTenantStore((s) => s.activeStore);
  const setActiveStore = useTenantStore((s) => s.setActiveStore);
  const setStores = useTenantStore((s) => s.setStores);
  const hydrated = useTenantStore((s) => s.hydrated);

  const { data: stores, isLoading, isError, error, refetch } = useStores();
  const [needsPicker, setNeedsPicker] = useState(false);
  const [decided, setDecided] = useState(false);

  useEffect(() => {
    if (!stores || activeStore || decided) return;
    setStores(stores);

    if (stores.length === 0) {
      setDecided(true);
      return;
    }

    if (stores.length === 1) {
      setActiveStore(stores[0]!);
      setDecided(true);
      return;
    }

    // 2+ stores: try to restore the last selection. If it isn't in the
    // membership list anymore (role revoked, store deleted), fall
    // through to the picker rather than silently defaulting.
    let cancelled = false;
    (async () => {
      const lastId = await tokenStorage.getStoreId().catch(() => null);
      if (cancelled) return;
      const match = lastId ? stores.find((s) => s.id === lastId) : undefined;
      if (match) {
        setActiveStore(match);
      } else {
        setNeedsPicker(true);
      }
      setDecided(true);
    })();

    return () => {
      cancelled = true;
    };
  }, [stores, activeStore, decided, setActiveStore, setStores]);

  // If something else clears the active store (membership revoked, user
  // signs out and back in as a different account), reset the decision
  // flags so the resolver runs again.
  useEffect(() => {
    if (!activeStore && decided && stores && stores.length > 0) {
      setDecided(false);
      setNeedsPicker(false);
    }
  }, [activeStore, decided, stores]);

  // Derive picker visibility from current state — once an active store
  // is chosen, the picker dismisses on the next render even if the
  // internal flag is still latched. Likewise, onboarding is only "now"
  // when we have stores data showing zero entries.
  const showPicker = needsPicker && !activeStore;
  const showOnboarding = !!stores && stores.length === 0 && !activeStore;

  return {
    resolving:
      !hydrated ||
      isLoading ||
      (!!stores && stores.length > 0 && !activeStore && !showPicker && !decided),
    needsOnboarding: showOnboarding,
    needsPicker: showPicker,
    availableStores: showPicker ? (stores ?? []) : [],
    error: isError ? error : null,
    refetch,
  };
}
