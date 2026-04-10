"use client";

// apps/admin/lib/products/useProductSelection.ts
//
// sessionStorage-backed selection state for the products list.
// Keyed by storeId so selecting on store A never leaks to store B.
// Hard cap of 100 ids — enforced here, in the Zod schema, and in
// the backend handler (three layers).

import { useState, useCallback, useEffect } from "react";

export const MAX_SELECTION = 100;

function storageKey(storeId: string): string {
  return `products.selection.${storeId}`;
}

function readStorage(storeId: string): string[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = sessionStorage.getItem(storageKey(storeId));
    if (!raw) return [];
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed.filter((x): x is string => typeof x === "string");
  } catch {
    return [];
  }
}

function writeStorage(storeId: string, ids: string[]): void {
  if (typeof window === "undefined") return;
  try {
    sessionStorage.setItem(storageKey(storeId), JSON.stringify(ids));
  } catch {
    // Storage full or unavailable — silent fallback
  }
}

export interface ProductSelection {
  selectedIds: string[];
  toggle: (id: string) => void;
  selectAll: (ids: string[]) => void;
  clearAll: () => void;
  isSelected: (id: string) => boolean;
  count: number;
}

export function useProductSelection(storeId: string): ProductSelection {
  const [selectedIds, setSelectedIds] = useState<string[]>(() =>
    readStorage(storeId),
  );

  // Rehydrate when storeId changes
  useEffect(() => {
    setSelectedIds(readStorage(storeId));
  }, [storeId]);

  // Sync to sessionStorage on every change
  useEffect(() => {
    writeStorage(storeId, selectedIds);
  }, [storeId, selectedIds]);

  const toggle = useCallback((id: string) => {
    setSelectedIds((prev) => {
      if (prev.includes(id)) {
        return prev.filter((x) => x !== id);
      }
      if (prev.length >= MAX_SELECTION) {
        return prev; // silently drop past cap
      }
      return [...prev, id];
    });
  }, []);

  const selectAll = useCallback((ids: string[]) => {
    setSelectedIds(ids.slice(0, MAX_SELECTION));
  }, []);

  const clearAll = useCallback(() => {
    setSelectedIds([]);
  }, []);

  const isSelected = useCallback(
    (id: string) => selectedIds.includes(id),
    [selectedIds],
  );

  return {
    selectedIds,
    toggle,
    selectAll,
    clearAll,
    isSelected,
    count: selectedIds.length,
  };
}
