import { StyleSheet } from 'react-native';
import { fireEvent, render } from '@testing-library/react-native';

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

  it('falls back to the placeholder when the image fails to load, at the same dimensions', () => {
    const { getByTestId } = render(
      <Thumb uri="https://cdn.example/a.jpg" testID="thumb" />,
    );
    const beforeStyle = StyleSheet.flatten(getByTestId('thumb').props.style);
    expect(getByTestId('thumb').props.contentFit).toBe('cover'); // image branch

    fireEvent(getByTestId('thumb'), 'error');

    // Placeholder branch: no contentFit prop (that's expo-image-only), same box.
    expect(getByTestId('thumb').props.contentFit).toBeUndefined();
    const afterStyle = StyleSheet.flatten(getByTestId('thumb').props.style);
    expect(afterStyle.width).toBe(beforeStyle.width);
    expect(afterStyle.height).toBe(beforeStyle.height);
    expect(afterStyle.width).toBe(theme.thumb.list);
    expect(afterStyle.height).toBe(theme.thumb.list);
  });

  it('clears a failure and retries when uri changes (FlatList recycling)', () => {
    const { getByTestId, rerender } = render(
      <Thumb uri="https://cdn.example/a.jpg" testID="thumb" />,
    );
    fireEvent(getByTestId('thumb'), 'error');
    expect(getByTestId('thumb').props.contentFit).toBeUndefined(); // now placeholder

    rerender(<Thumb uri="https://cdn.example/b.jpg" testID="thumb" />);

    expect(getByTestId('thumb').props.contentFit).toBe('cover');
    expect(getByTestId('thumb').props.source).toEqual({
      uri: 'https://cdn.example/b.jpg',
    });
  });
});
