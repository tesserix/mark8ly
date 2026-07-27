import type { ComponentType } from 'react';
import { Pressable, Text as RNText, StyleSheet, type StyleProp, type ViewStyle } from 'react-native';
import { render, fireEvent } from '@testing-library/react-native';
import { PressableRow } from '@/components/ui/PressableRow';
import { theme } from '@/lib/theme';

describe('PressableRow', () => {
  it('renders its children', () => {
    const { getByText } = render(
      <PressableRow onPress={() => {}} accessibilityLabel="Order 1042" testID="row">
        <RNText>Order 1042</RNText>
      </PressableRow>,
    );
    expect(getByText('Order 1042')).toBeTruthy();
  });

  it('calls onPress when tapped', () => {
    const onPress = jest.fn();
    const { getByTestId } = render(
      <PressableRow onPress={onPress} accessibilityLabel="Order 1042" testID="row">
        <RNText>Order 1042</RNText>
      </PressableRow>,
    );
    fireEvent.press(getByTestId('row'));
    expect(onPress).toHaveBeenCalledTimes(1);
  });

  it('uses the single-line minimum height by default', () => {
    const { getByTestId } = render(
      <PressableRow onPress={() => {}} accessibilityLabel="Row" testID="row">
        <RNText>Row</RNText>
      </PressableRow>,
    );
    const style = StyleSheet.flatten(getByTestId('row').props.style);
    expect(style.minHeight).toBe(theme.row.minHeightSingle);
    expect(style.paddingHorizontal).toBe(theme.row.paddingH);
    expect(style.paddingVertical).toBe(theme.row.paddingV);
  });

  it('uses the two-line minimum height when lines is 2', () => {
    const { getByTestId } = render(
      <PressableRow onPress={() => {}} lines={2} accessibilityLabel="Row" testID="row">
        <RNText>Row</RNText>
      </PressableRow>,
    );
    const style = StyleSheet.flatten(getByTestId('row').props.style);
    expect(style.minHeight).toBe(theme.row.minHeightDouble);
  });

  it('never sets an opacity-based press style', () => {
    const { getByTestId } = render(
      <PressableRow onPress={() => {}} accessibilityLabel="Row" testID="row">
        <RNText>Row</RNText>
      </PressableRow>,
    );
    const style = StyleSheet.flatten(getByTestId('row').props.style);
    expect(style.opacity).toBeUndefined();
  });

  // RN's <Pressable> only materializes `android_ripple` onto the underlying
  // host node when Platform.OS === 'android' (see
  // useAndroidRippleForView.js) — under jest-expo, Platform.OS defaults to
  // 'ios', so `getByTestId('row').props.android_ripple` is always
  // `undefined` here regardless of what PressableRow passes in. Asserting
  // on the <Pressable> element itself verifies the same intent — that
  // PressableRow wires up the 12%-ink ripple — without depending on RN's
  // platform-conditional host-node materialization. `Pressable` is exported
  // as `memo(Pressable)`, so `UNSAFE_getByType(Pressable)` can't match the
  // fiber (react-test-renderer matches the memo's inner render function);
  // unwrap it via `Pressable.type` to reach the same instance.
  it('exposes an Android ripple at 12% ink', () => {
    const { UNSAFE_getByType } = render(
      <PressableRow onPress={() => {}} accessibilityLabel="Row" testID="row">
        <RNText>Row</RNText>
      </PressableRow>,
    );
    const innerPressableType = (Pressable as unknown as { type: ComponentType<unknown> }).type;
    expect(UNSAFE_getByType(innerPressableType).props.android_ripple).toEqual({
      color: 'rgba(14, 14, 12, 0.12)',
    });
  });

  // Regression for a real bug: PressableRow used to order its style array
  // [base, lines, pressed, style] — RN flattens later-wins, so any caller
  // that passes an explicit `backgroundColor` in `style` (StorePicker,
  // DashboardOrderRow, CampaignRow, and every other row sitting on a
  // Card/sheet surface, all of which correctly override the paper default)
  // silently killed the iOS press feedback entirely. Android still rippled,
  // which is why it looked fine on emulator. Drive the style FUNCTION
  // directly rather than real press-state, per jest-expo's Platform.OS
  // pin to 'ios' (see the ripple test above) and because simulating a real
  // pressIn/pressOut cycle through Pressable's internal state is fragile
  // under react-test-renderer.
  it('caller-supplied backgroundColor never wins over the pressed sink state', () => {
    const { UNSAFE_getByType } = render(
      <PressableRow
        onPress={() => {}}
        accessibilityLabel="Row"
        testID="row"
        style={{ backgroundColor: theme.colors.elevated }}
      >
        <RNText>Row</RNText>
      </PressableRow>,
    );
    const innerPressableType = (Pressable as unknown as { type: ComponentType<unknown> }).type;
    const styleFn = UNSAFE_getByType(innerPressableType).props.style as (state: {
      pressed: boolean;
    }) => StyleProp<ViewStyle>;

    // Unpressed: the caller's elevated background wins, matching its parent
    // Card/sheet surface instead of PressableRow's paper default.
    expect(StyleSheet.flatten(styleFn({ pressed: false })).backgroundColor).toBe(
      theme.colors.elevated,
    );
    // Pressed: the row's sink press state MUST win over the caller's
    // backgroundColor override — this is the exact regression the ordering
    // bug produced.
    expect(StyleSheet.flatten(styleFn({ pressed: true })).backgroundColor).toBe(
      theme.colors.sink,
    );
  });

  it('forwards onLongPress', () => {
    const onLongPress = jest.fn();
    const { getByTestId } = render(
      <PressableRow
        onPress={() => {}}
        onLongPress={onLongPress}
        accessibilityLabel="Row"
        testID="row"
      >
        <RNText>Row</RNText>
      </PressableRow>,
    );
    fireEvent(getByTestId('row'), 'longPress');
    expect(onLongPress).toHaveBeenCalledTimes(1);
  });
});
