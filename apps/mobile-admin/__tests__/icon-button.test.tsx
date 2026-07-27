import type { ComponentType } from 'react';
import { Pressable, Text as RNText, StyleSheet } from 'react-native';
import { render, fireEvent } from '@testing-library/react-native';
import { IconButton } from '@/components/ui/IconButton';
import { theme } from '@/lib/theme';

// `Pressable` is exported as `memo(Pressable)`, so `UNSAFE_getByType(Pressable)`
// can't match the fiber (react-test-renderer matches the memo's inner render
// function) — unwrap it via `Pressable.type` to reach the same instance, same
// pattern as pressable-row.test.tsx.
const innerPressableType = (Pressable as unknown as { type: ComponentType<unknown> }).type;

describe('IconButton', () => {
  it('renders its children', () => {
    const { getByTestId } = render(
      <IconButton onPress={() => {}} accessibilityLabel="Go back" testID="icon-btn">
        <RNText>×</RNText>
      </IconButton>,
    );
    expect(getByTestId('icon-btn')).toBeTruthy();
  });

  it('calls onPress when tapped', () => {
    const onPress = jest.fn();
    const { getByTestId } = render(
      <IconButton onPress={onPress} accessibilityLabel="Go back" testID="icon-btn">
        <RNText>×</RNText>
      </IconButton>,
    );
    fireEvent.press(getByTestId('icon-btn'));
    expect(onPress).toHaveBeenCalledTimes(1);
  });

  // Core requirement: the touch target is a REAL minWidth/minHeight on the
  // Pressable itself, not an invisible hitSlop overlay that can bleed onto
  // adjacent content.
  it('enforces the 44pt minimum via minWidth/minHeight, not hitSlop', () => {
    const { getByTestId } = render(
      <IconButton onPress={() => {}} accessibilityLabel="Go back" testID="icon-btn">
        <RNText>×</RNText>
      </IconButton>,
    );
    const node = getByTestId('icon-btn');
    const style = StyleSheet.flatten(node.props.style);
    expect(style.minWidth).toBe(theme.touchTarget);
    expect(style.minHeight).toBe(theme.touchTarget);
    expect(node.props.hitSlop).toBeUndefined();
  });

  // Guards the exact NativeWind interop bug that shipped in increment 1: a
  // function style prop silently drops the base styles at runtime. If someone
  // "simplifies" IconButton back to `style={({pressed}) => …}`, this fails.
  it('passes a plain array style, never a function', () => {
    const { getByTestId } = render(
      <IconButton onPress={() => {}} accessibilityLabel="Go back" testID="icon-btn">
        <RNText>×</RNText>
      </IconButton>,
    );
    expect(typeof getByTestId('icon-btn').props.style).not.toBe('function');
  });

  it('merges a caller-supplied style into the final array', () => {
    const { getByTestId } = render(
      <IconButton
        onPress={() => {}}
        accessibilityLabel="Go back"
        testID="icon-btn"
        style={{ marginLeft: 12 }}
      >
        <RNText>×</RNText>
      </IconButton>,
    );
    const style = StyleSheet.flatten(getByTestId('icon-btn').props.style);
    expect(style.marginLeft).toBe(12);
    // Caller style must not be able to shrink the touch target below the
    // 44pt minimum — minWidth/minHeight must still be present.
    expect(style.minWidth).toBe(theme.touchTarget);
    expect(style.minHeight).toBe(theme.touchTarget);
  });

  it('defaults accessibilityRole to "button" and allows an override', () => {
    const { getByTestId } = render(
      <IconButton
        onPress={() => {}}
        accessibilityLabel="Go back"
        accessibilityRole="link"
        testID="icon-btn"
      >
        <RNText>×</RNText>
      </IconButton>,
    );
    expect(getByTestId('icon-btn').props.accessibilityRole).toBe('link');
  });

  it('defaults accessibilityRole to "button" when not overridden', () => {
    const { getByTestId } = render(
      <IconButton onPress={() => {}} accessibilityLabel="Go back" testID="icon-btn">
        <RNText>×</RNText>
      </IconButton>,
    );
    expect(getByTestId('icon-btn').props.accessibilityRole).toBe('button');
  });

  describe('tone → ripple token', () => {
    it('defaults to "ink" → rippleInk', () => {
      const { UNSAFE_getByType } = render(
        <IconButton onPress={() => {}} accessibilityLabel="Go back">
          <RNText>×</RNText>
        </IconButton>,
      );
      expect(UNSAFE_getByType(innerPressableType).props.android_ripple).toEqual({
        ...theme.press.rippleInk,
        borderless: true,
      });
    });

    it('maps tone="onDark" → rippleOnDark', () => {
      const { UNSAFE_getByType } = render(
        <IconButton onPress={() => {}} accessibilityLabel="Remove image 1" tone="onDark">
          <RNText>×</RNText>
        </IconButton>,
      );
      expect(UNSAFE_getByType(innerPressableType).props.android_ripple).toEqual({
        ...theme.press.rippleOnDark,
        borderless: true,
      });
    });

    it('maps tone="danger" → rippleDanger', () => {
      const { UNSAFE_getByType } = render(
        <IconButton onPress={() => {}} accessibilityLabel="Revoke invite" tone="danger">
          <RNText>×</RNText>
        </IconButton>,
      );
      expect(UNSAFE_getByType(innerPressableType).props.android_ripple).toEqual({
        ...theme.press.rippleDanger,
        borderless: true,
      });
    });

    it('maps tone="accent" → rippleAccent', () => {
      const { UNSAFE_getByType } = render(
        <IconButton onPress={() => {}} accessibilityLabel="Accent action" tone="accent">
          <RNText>×</RNText>
        </IconButton>,
      );
      expect(UNSAFE_getByType(innerPressableType).props.android_ripple).toEqual({
        ...theme.press.rippleAccent,
        borderless: true,
      });
    });
  });

  describe('iOS opacity dim', () => {
    // jest-expo pins Platform.OS to 'ios', so the pressed branch is live here.
    it('uses opacityStandard for a transparent-glyph tone (ink)', () => {
      const { getByTestId } = render(
        <IconButton onPress={() => {}} accessibilityLabel="Go back" testID="icon-btn">
          <RNText>×</RNText>
        </IconButton>,
      );
      fireEvent(getByTestId('icon-btn'), 'pressIn');
      const style = StyleSheet.flatten(getByTestId('icon-btn').props.style);
      expect(style.opacity).toBe(theme.press.opacityStandard);
    });

    // tone="onDark" pairs with a solid ink/moss fill background (e.g. the
    // products FAB, the media-picker remove badge) — opacityStandard's 45%
    // fade "looks broken, not pressed" on a filled surface per theme.ts, so
    // this tone uses the gentler opacitySolidFill instead, matching what
    // every pre-migration onDark call site already did.
    it('uses opacitySolidFill for tone="onDark"', () => {
      const { getByTestId } = render(
        <IconButton onPress={() => {}} accessibilityLabel="Remove image 1" tone="onDark" testID="icon-btn">
          <RNText>×</RNText>
        </IconButton>,
      );
      fireEvent(getByTestId('icon-btn'), 'pressIn');
      const style = StyleSheet.flatten(getByTestId('icon-btn').props.style);
      expect(style.opacity).toBe(theme.press.opacitySolidFill);
    });

    it('clears the opacity dim on pressOut', () => {
      const { getByTestId } = render(
        <IconButton onPress={() => {}} accessibilityLabel="Go back" testID="icon-btn">
          <RNText>×</RNText>
        </IconButton>,
      );
      fireEvent(getByTestId('icon-btn'), 'pressIn');
      fireEvent(getByTestId('icon-btn'), 'pressOut');
      const style = StyleSheet.flatten(getByTestId('icon-btn').props.style);
      expect(style.opacity).toBeUndefined();
    });
  });

  describe('disabled', () => {
    it("sets Pressable's own disabled prop", () => {
      const { UNSAFE_getByType } = render(
        <IconButton onPress={() => {}} accessibilityLabel="Go back" disabled>
          <RNText>×</RNText>
        </IconButton>,
      );
      expect(UNSAFE_getByType(innerPressableType).props.disabled).toBe(true);
    });

    it('reaches accessibilityState.disabled', () => {
      const { getByTestId } = render(
        <IconButton onPress={() => {}} accessibilityLabel="Go back" disabled testID="icon-btn">
          <RNText>×</RNText>
        </IconButton>,
      );
      expect(getByTestId('icon-btn').props.accessibilityState).toEqual(
        expect.objectContaining({ disabled: true }),
      );
    });

    it('does not fire onPress when disabled', () => {
      const onPress = jest.fn();
      const { getByTestId } = render(
        <IconButton onPress={onPress} accessibilityLabel="Go back" disabled testID="icon-btn">
          <RNText>×</RNText>
        </IconButton>,
      );
      fireEvent.press(getByTestId('icon-btn'));
      expect(onPress).not.toHaveBeenCalled();
    });

    it('suppresses the opacity dim: pressIn never engages it while disabled', () => {
      const { getByTestId } = render(
        <IconButton onPress={() => {}} accessibilityLabel="Go back" disabled testID="icon-btn">
          <RNText>×</RNText>
        </IconButton>,
      );
      fireEvent(getByTestId('icon-btn'), 'pressIn');
      const style = StyleSheet.flatten(getByTestId('icon-btn').props.style);
      expect(style.opacity).toBeUndefined();
    });

    it('clears the android ripple while disabled', () => {
      const { UNSAFE_getByType } = render(
        <IconButton onPress={() => {}} accessibilityLabel="Go back" disabled>
          <RNText>×</RNText>
        </IconButton>,
      );
      expect(UNSAFE_getByType(innerPressableType).props.android_ripple).toBeUndefined();
    });
  });
});
