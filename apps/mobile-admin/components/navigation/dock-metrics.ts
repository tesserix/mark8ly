import { useSafeAreaInsets } from "react-native-safe-area-context";

/** Floating dock bar height. */
export const DOCK_HEIGHT = 64;

/** Gap between the dock and the bottom safe-area inset. */
export const DOCK_BOTTOM_GAP = 4;

/**
 * Bottom padding a scroll view needs so its last row clears the floating
 * dock (dock height + gap + safe-area inset + a breathing row).
 */
export function useDockClearance(): number {
  const insets = useSafeAreaInsets();
  return insets.bottom + DOCK_BOTTOM_GAP + DOCK_HEIGHT + 12;
}
