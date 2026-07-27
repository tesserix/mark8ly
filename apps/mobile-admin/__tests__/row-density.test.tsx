import { StyleSheet } from 'react-native';
import { render } from '@testing-library/react-native';

jest.mock('expo-image', () => {
  const { View } = require('react-native');
  return { Image: View };
});

// The `@/components/ui` barrel re-exports SearchField (and Thumb's own
// placeholder), both of which import lucide-react-native's ESM build at
// module load time — unparseable under jest regardless of which icon
// actually renders. Same landmine as __tests__/customer-row.test.tsx and
// __tests__/thumb.test.tsx.
jest.mock('lucide-react-native', () => new Proxy({}, { get: () => () => null }));

import { ProductRow } from '@/components/ProductRow';
import { theme } from '@/lib/theme';
import type { Product } from '@repo/mobile-shared/api/types';

const product = {
  id: 'p1',
  title: 'Bondi Linen Shirt',
  status: 'active',
  variants: [],
  media: [],
} as unknown as Product;

describe('list row density', () => {
  it('renders ProductRow at the two-line native height', () => {
    const { getByTestId } = render(
      <ProductRow product={product} onPress={() => {}} />,
    );
    const style = StyleSheet.flatten(getByTestId('product-row-p1').props.style);
    expect(style.minHeight).toBe(theme.row.minHeightDouble);
    expect(style.paddingHorizontal).toBe(theme.row.paddingH);
  });

  it('does not apply an opacity press style', () => {
    const { getByTestId } = render(
      <ProductRow product={product} onPress={() => {}} />,
    );
    const style = StyleSheet.flatten(getByTestId('product-row-p1').props.style);
    expect(style.opacity).toBeUndefined();
  });
});
