import { StyleSheet } from 'react-native';
import { render } from '@testing-library/react-native';

jest.mock('expo-image', () => {
  const { View } = require('react-native');
  return { Image: View };
});

// Thumb's placeholder renders the Package icon from lucide-react-native's
// ESM build, which jest-expo's default transform doesn't cover (same
// landmine every other lucide-consuming test in this suite works around —
// see e.g. __tests__/customer-row.test.tsx).
jest.mock('lucide-react-native', () => new Proxy({}, { get: () => () => null }));

import { Thumb } from '@/components/ui/Thumb';
import { theme } from '@/lib/theme';

describe('Thumb', () => {
  it('renders at the list size by default', () => {
    const { getByTestId } = render(
      <Thumb uri="https://cdn.example/a.jpg" testID="thumb" />,
    );
    const style = StyleSheet.flatten(getByTestId('thumb').props.style);
    expect(style.width).toBe(theme.thumb.list);
    expect(style.height).toBe(theme.thumb.list);
  });

  it('renders at the compact size when asked', () => {
    const { getByTestId } = render(
      <Thumb uri="https://cdn.example/a.jpg" size="compact" testID="thumb" />,
    );
    const style = StyleSheet.flatten(getByTestId('thumb').props.style);
    expect(style.width).toBe(theme.thumb.compact);
  });

  it('renders the placeholder at identical dimensions when uri is missing', () => {
    const { getByTestId } = render(<Thumb testID="thumb" />);
    const style = StyleSheet.flatten(getByTestId('thumb').props.style);
    expect(style.width).toBe(theme.thumb.list);
    expect(style.height).toBe(theme.thumb.list);
  });

  it('renders the placeholder when uri is null', () => {
    const { getByTestId } = render(<Thumb uri={null} testID="thumb" />);
    expect(getByTestId('thumb')).toBeTruthy();
  });

  it('sets a 200ms transition and a recycling key on the image', () => {
    const { getByTestId } = render(
      <Thumb uri="https://cdn.example/a.jpg" recyclingKey="prod-1" testID="thumb" />,
    );
    expect(getByTestId('thumb').props.transition).toBe(200);
    expect(getByTestId('thumb').props.recyclingKey).toBe('prod-1');
    expect(getByTestId('thumb').props.contentFit).toBe('cover');
  });
});
