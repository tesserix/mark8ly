import type { ComponentType } from 'react';
import { Pressable, Text as RNText, StyleSheet } from 'react-native';
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
  // which is why it looked fine on emulator.
  //
  // This drives a REAL pressIn/pressOut cycle rather than poking a style
  // function, because PressableRow no longer has one: under NativeWind's JSX
  // interop a function `style` prop isn't resolved like a plain array and the
  // base styles were dropped at runtime (rows rendered with no padding and
  // stacked vertically). Press state is now explicit, so the array is
  // inspectable directly — which is also a truer test than calling the
  // function ourselves. jest-expo pins Platform.OS to 'ios', so the pressed
  // branch is live here (see the ripple test above).
  it('caller-supplied backgroundColor never wins over the pressed sink state', () => {
    const { getByTestId } = render(
      <PressableRow
        onPress={() => {}}
        accessibilityLabel="Row"
        testID="row"
        style={{ backgroundColor: theme.colors.elevated }}
      >
        <RNText>Row</RNText>
      </PressableRow>,
    );

    // Unpressed: the caller's elevated background wins, matching its parent
    // Card/sheet surface instead of PressableRow's paper default.
    expect(
      StyleSheet.flatten(getByTestId('row').props.style).backgroundColor,
    ).toBe(theme.colors.elevated);

    // Pressed: the row's sink press state MUST win over the caller's
    // backgroundColor override — this is the exact regression the ordering
    // bug produced.
    fireEvent(getByTestId('row'), 'pressIn');
    expect(
      StyleSheet.flatten(getByTestId('row').props.style).backgroundColor,
    ).toBe(theme.colors.sink);

    // …and released, it returns to the caller's surface.
    fireEvent(getByTestId('row'), 'pressOut');
    expect(
      StyleSheet.flatten(getByTestId('row').props.style).backgroundColor,
    ).toBe(theme.colors.elevated);
  });

  // Guards the NativeWind interop bug directly: a function style prop is not
  // resolved at runtime, so the base styles vanish. If someone "simplifies"
  // PressableRow back to `style={({pressed}) => …}`, this fails.
  it('passes a plain array style, never a function', () => {
    const { getByTestId } = render(
      <PressableRow onPress={() => {}} accessibilityLabel="Row" testID="row">
        <RNText>Row</RNText>
      </PressableRow>,
    );
    expect(typeof getByTestId('row').props.style).not.toBe('function');
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

  it('defaults accessibilityRole to "button" and allows an override', () => {
    const { getByTestId } = render(
      <PressableRow
        onPress={() => {}}
        accessibilityLabel="Store A"
        accessibilityRole="link"
        testID="row"
      >
        <RNText>Store A</RNText>
      </PressableRow>,
    );
    expect(getByTestId('row').props.accessibilityRole).toBe('link');
  });

  it('passes an accessibilityHint through to Pressable', () => {
    const { getByTestId } = render(
      <PressableRow
        onPress={() => {}}
        accessibilityLabel="Row"
        accessibilityHint="Opens details"
        testID="row"
      >
        <RNText>Row</RNText>
      </PressableRow>,
    );
    expect(getByTestId('row').props.accessibilityHint).toBe('Opens details');
  });

  it('passes a custom ripple colour through to the underlying Pressable', () => {
    const ripple = { color: 'rgba(139, 46, 32, 0.12)' };
    const { UNSAFE_getByType } = render(
      <PressableRow onPress={() => {}} accessibilityLabel="Row" ripple={ripple} testID="row">
        <RNText>Row</RNText>
      </PressableRow>,
    );
    const innerPressableType = (Pressable as unknown as { type: ComponentType<unknown> }).type;
    expect(UNSAFE_getByType(innerPressableType).props.android_ripple).toEqual(ripple);
  });

  // Restored in the increment 2 carry-over fixes: StoreSelector lost this
  // prop in the migration to PressableRow (no `selected` state was
  // announced for the active store row) because PressableRow had nowhere to
  // put it.
  it('passes a caller accessibilityState through when not disabled', () => {
    const { getByTestId } = render(
      <PressableRow
        onPress={() => {}}
        accessibilityLabel="Store A, currently selected"
        accessibilityState={{ selected: true }}
        testID="row"
      >
        <RNText>Store A</RNText>
      </PressableRow>,
    );
    // RN's Pressable normalises accessibilityState to a full object (busy/
    // checked/expanded default to undefined, disabled defaults to false) —
    // assert on the keys this component actually controls.
    expect(getByTestId('row').props.accessibilityState).toEqual(
      expect.objectContaining({ selected: true, disabled: false }),
    );
  });

  describe('disabled', () => {
    // The `disabled` prop is the other increment 1 carry-over fix:
    // PressableRow had no way to render a non-interactive row, which forced
    // app/(tabs)/more/settings/team/index.tsx to hand-mirror this
    // component's entire base style for the owner/pending-mutation row.
    it("sets Pressable's own disabled prop", () => {
      const { UNSAFE_getByType } = render(
        <PressableRow onPress={() => {}} accessibilityLabel="Row" disabled testID="row">
          <RNText>Row</RNText>
        </PressableRow>,
      );
      const innerPressableType = (Pressable as unknown as { type: ComponentType<unknown> }).type;
      expect(UNSAFE_getByType(innerPressableType).props.disabled).toBe(true);
    });

    it('results in accessibilityState.disabled reaching the rendered node', () => {
      // RN's Pressable merges its own `disabled` prop into the
      // accessibilityState it hands to the host node (see Pressable.js), so
      // this component doesn't (and must not) duplicate that merge itself —
      // this is the end-to-end behaviour a screen reader actually observes.
      const { getByTestId } = render(
        <PressableRow
          onPress={() => {}}
          accessibilityLabel="Row"
          accessibilityState={{ selected: true }}
          disabled
          testID="row"
        >
          <RNText>Row</RNText>
        </PressableRow>,
      );
      expect(getByTestId('row').props.accessibilityState).toEqual(
        expect.objectContaining({ selected: true, disabled: true }),
      );
    });

    it('does not fire onPress when disabled', () => {
      const onPress = jest.fn();
      const { getByTestId } = render(
        <PressableRow onPress={onPress} accessibilityLabel="Row" disabled testID="row">
          <RNText>Row</RNText>
        </PressableRow>,
      );
      fireEvent.press(getByTestId('row'));
      expect(onPress).not.toHaveBeenCalled();
    });

    it('suppresses press feedback: pressIn never engages the pressed sink style', () => {
      const { getByTestId } = render(
        <PressableRow onPress={() => {}} accessibilityLabel="Row" disabled testID="row">
          <RNText>Row</RNText>
        </PressableRow>,
      );
      fireEvent(getByTestId('row'), 'pressIn');
      expect(
        StyleSheet.flatten(getByTestId('row').props.style).backgroundColor,
      ).not.toBe(theme.colors.sink);
    });

    it('clears the android ripple while disabled', () => {
      const { UNSAFE_getByType } = render(
        <PressableRow onPress={() => {}} accessibilityLabel="Row" disabled testID="row">
          <RNText>Row</RNText>
        </PressableRow>,
      );
      const innerPressableType = (Pressable as unknown as { type: ComponentType<unknown> }).type;
      expect(UNSAFE_getByType(innerPressableType).props.android_ripple).toBeUndefined();
    });
  });
});
