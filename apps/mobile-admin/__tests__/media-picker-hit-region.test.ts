// ProductMediaPicker transitively imports lucide-react-native (ESM build jest
// can't parse) and expo-image — the established mock pattern in this repo.
jest.mock('lucide-react-native', () => new Proxy({}, { get: () => () => null }));
jest.mock('expo-image', () => {
  const { View } = require('react-native');
  return { Image: View };
});

import { REMOVE_BTN_GEOMETRY as G } from '@/components/ProductMediaPicker';

/**
 * The media picker's remove badge sits in the top-right corner of an 80pt
 * thumbnail, offset outward, with a hitSlop-expanded tap region. The next
 * thumbnail is only `gallery`'s gap away.
 *
 * If the expanded region ever reaches the neighbour, tapping near image N+1
 * deletes image N — a destructive, silent regression. An earlier version of
 * this badge inherited IconButton's 44pt box and overlapped the neighbour by
 * ~6pt; this asserts the invariant so the arithmetic can't drift again.
 *
 * All coordinates are thumbWrap-local, x increasing rightward, origin at the
 * thumbnail's top-left.
 */
describe('media picker remove badge hit region', () => {
  // Badge is offset OUTWARD from the thumb's right edge.
  const badgeRight = G.thumbSize + G.badgeOffset;
  const hitRegionRight = badgeRight + G.hitSlop.right;
  const neighbourLeft = G.thumbSize + G.siblingGap;

  it('never reaches the neighbouring thumbnail', () => {
    expect(hitRegionRight).toBeLessThan(neighbourLeft);
  });

  it('reports the clearance so a shrinking gap fails loudly', () => {
    // Not a magic number — recompute if the geometry legitimately changes,
    // but recompute the OVERLAP too. 8 - (6 + 1) = 1.
    expect(neighbourLeft - hitRegionRight).toBe(1);
  });

  it('still reaches a 44pt tap target overall', () => {
    const width = G.badgeSize + G.hitSlop.left + G.hitSlop.right;
    const height = G.badgeSize + G.hitSlop.top + G.hitSlop.bottom;
    expect(width).toBeGreaterThanOrEqual(44);
    expect(height).toBeGreaterThanOrEqual(44);
  });

  it('expands away from the neighbour, not toward it', () => {
    // The slop is deliberately asymmetric: generous into the thumbnail's own
    // interior, minimal toward the next photo.
    expect(G.hitSlop.left).toBeGreaterThan(G.hitSlop.right);
  });
});
