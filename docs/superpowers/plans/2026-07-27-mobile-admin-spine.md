# mobile-admin Native UX — Increment 1 (Spine + API) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Re-scale mobile-admin to native type and density metrics, replace every opacity-fade press with platform-native press feedback, add the admin haptics set and `expo-image` thumbnails — and add the three dashboard API fields the later increments need.

**Architecture:** Two independent workstreams. The **spine** is a global, mechanical pass over `apps/mobile-admin`: tokens first (type scale, density, `sink` colour), then two new shared primitives (`PressableRow`, `Thumb`), then every call site migrated onto them. The **API** workstream adds three fields to the marketplace-api dashboard DTO plus one bug-fix field, and widens the zod schema to accept them. Neither blocks the other; the client renders correctly with or without the new fields.

**Tech Stack:** React Native 0.85.3 · Expo SDK 56 · expo-router 56 · NativeWind 4.2.5 + Tailwind 3.4 · reanimated 4.3 · gesture-handler 2.31 · expo-image · expo-haptics · jest-expo (mobile-admin) · vitest (mobile-shared) · Go 1.26 + Gin + GORM (marketplace-api)

## Global Constraints

- **Zero new npm dependencies.** Every package needed is already installed. The root lockfile cannot be regenerated locally (`npm install` collapses the deliberate jest-29+30 / RN-0.76+0.85 multi-version tree and breaks mobile-admin). If a task appears to need a lockfile edit, that task is wrong — stop and report.
- **Two token sources must stay in agreement:** `apps/mobile-admin/lib/theme.ts` and `apps/mobile-admin/tailwind.config.js`. Every type or colour change lands in both.
- **Banned text colours — never reintroduce:** `rgba(14, 14, 12, 0.5)` and `#7A766E`. Both fail WCAG AA 4.5:1 on Paper `#F7F6F2`. Tertiary text is `#5C5953` in both sources.
- **One accent per view.** Moss `#2D4A2B` is never decorative.
- **Press feedback rule (resolved 2026-07-27).** `TouchableOpacity` and `activeOpacity` appear
  nowhere in the codebase when this plan is done. The ban on opacity press feedback applies to
  **rows and other surfaces that have a background to shift** — those use the sink colour on
  iOS and a ripple on Android. **Icon buttons are the documented exception:** a transparent
  24 pt glyph has no background to shift, so a brief `opacity: 0.55` while pressed is the
  correct native behaviour, not a leftover. This resolves the apparent conflict between spec
  §1.3 and Task 7.
- **Badges:** success is moss *tint* (`#E8EEE2` bg / `#2D4A2B` fg), never a solid moss fill. Warning is `#7A4A0F` on `#F4E6CB`, never white-on-amber.
- **No glassmorphism, no centered heroes, no dark mode.**
- **Minimum touch target 44 pt** (`theme.touchTarget`) — unchanged.
- **Radii:** `theme.radii.md` and tailwind `rounded-md` both stay at `6px`.
- **Gates that must stay green at every commit:** `npm test` in `apps/mobile-admin` (352 tests / 48 suites at baseline), `npm test` in `packages/mobile-shared` (83 tests), and `npm run check-types` in both.
- **Go `omitempty` pointers are ABSENT from JSON, not null.** New zod fields are `.optional()`, never `.nullable()`.
- **Commit style:** conventional commits, single-line messages, no signatures, no co-author trailers.

---

## File Structure

**Created**

| File | Responsibility |
|---|---|
| `apps/mobile-admin/components/ui/PressableRow.tsx` | The one row press surface: platform-adaptive feedback, density, long-press hook |
| `apps/mobile-admin/components/ui/Thumb.tsx` | expo-image thumbnail with placeholder and failure state |
| `apps/mobile-admin/__tests__/theme-tokens.test.ts` | Type-scale and density token assertions + banned-colour guard |
| `apps/mobile-admin/__tests__/pressable-row.test.tsx` | PressableRow behaviour |
| `apps/mobile-admin/__tests__/thumb.test.tsx` | Thumb behaviour |
| `packages/mobile-shared/haptics/__tests__/admin-feedback.test.ts` | Admin haptics triggers |

**Modified**

| File | Change |
|---|---|
| `apps/mobile-admin/lib/theme.ts` | Type scale, `row`/`thumb` density blocks, `colors.sink` |
| `apps/mobile-admin/tailwind.config.js` | Matching `fontSize` scale + `hero` size |
| `apps/mobile-admin/components/ui/Text.tsx` | `heroNumeral` preset |
| `apps/mobile-admin/components/ui/Text.test.tsx` | New preset assertions |
| `apps/mobile-admin/components/ui/index.ts` | Export the two new primitives |
| `packages/mobile-shared/haptics/feedback.ts` | `adminHaptics` export |
| 41 files using `TouchableOpacity` | Migrated to `Pressable` / `PressableRow` |
| `services/marketplace-api/internal/handlers/admin/dashboard.go` | 4 DTO fields + 2 query changes |
| `packages/mobile-shared/api/schemas/dashboard.ts` | 4 optional fields |

---

### Task 1: Type scale tokens

**Files:**
- Modify: `apps/mobile-admin/lib/theme.ts:125-190` (the `text` block)
- Modify: `apps/mobile-admin/tailwind.config.js:68-78` (the `fontSize` block)
- Modify: `apps/mobile-admin/components/ui/Text.tsx:3-26` (preset union + classes)
- Test: `apps/mobile-admin/components/ui/Text.test.tsx`
- Test: `apps/mobile-admin/__tests__/theme-tokens.test.ts` (create)

**Interfaces:**
- Consumes: nothing.
- Produces: `theme.text.heroNumeral`, and the `TextPreset` union in both `lib/theme.ts` (`keyof typeof theme.text`) and `components/ui/Text.tsx` now includes `'heroNumeral'`. Every later task renders body copy at 17 pt without passing a size.

- [ ] **Step 1: Write the failing tests**

Create `apps/mobile-admin/__tests__/theme-tokens.test.ts`:

```ts
import { theme } from '@/lib/theme';

describe('type scale — native metrics', () => {
  const expected = {
    heroNumeral: [44, 48],
    display: [40, 46],
    h1: [30, 36],
    h2: [24, 30],
    h3: [20, 26],
    bodyLg: [19, 26],
    body: [17, 24],
    bodyEmphasis: [17, 24],
    label: [15, 20],
    caption: [13, 18],
    eyebrow: [12, 16],
    mono: [15, 20],
  } as const;

  for (const [preset, [size, lineHeight]] of Object.entries(expected)) {
    it(`sets ${preset} to ${size}/${lineHeight}`, () => {
      const style = theme.text[preset as keyof typeof theme.text];
      expect(style.fontSize).toBe(size);
      expect(style.lineHeight).toBe(lineHeight);
    });
  }

  it('keeps bodyEmphasis at semibold', () => {
    expect(theme.text.bodyEmphasis.fontWeight).toBe('600');
  });
});
```

Append to `apps/mobile-admin/components/ui/Text.test.tsx`, inside the existing `describe('Text', ...)`:

```tsx
  it('maps the heroNumeral preset to the serif hero classes', () => {
    const { getByText } = render(<Text preset="heroNumeral">$4,280</Text>);
    const node = getByText('$4,280');
    expect(node.props.className).toContain('font-serif-bold');
    expect(node.props.className).toContain('text-hero');
    expect(node.props.className).toContain('text-ink');
  });
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
cd apps/mobile-admin && npx jest __tests__/theme-tokens.test.ts components/ui/Text.test.tsx
```

Expected: FAIL. `theme-tokens.test.ts` fails on `heroNumeral` being undefined and on every size assertion; `Text.test.tsx` fails because `text-hero` is not in the class string.

- [ ] **Step 3: Update `lib/theme.ts`**

Replace the whole `text: { ... }` block (currently `lib/theme.ts:125-190`) with:

```ts
  text: {
    // Home revenue figure only — kept separate so `display` can stay at 40
    // everywhere it is already used.
    heroNumeral: {
      fontFamily: serif,
      fontSize: 44,
      lineHeight: 48,
      fontWeight: "700",
      letterSpacing: -0.8,
    } satisfies TextStyle,
    display: {
      fontFamily: serif,
      fontSize: 40,
      lineHeight: 46,
      fontWeight: "700",
      letterSpacing: -0.6,
    } satisfies TextStyle,
    h1: {
      fontFamily: serif,
      fontSize: 30,
      lineHeight: 36,
      fontWeight: "700",
      letterSpacing: -0.4,
    } satisfies TextStyle,
    h2: {
      fontFamily: serif,
      fontSize: 24,
      lineHeight: 30,
      fontWeight: "700",
      letterSpacing: -0.25,
    } satisfies TextStyle,
    h3: {
      fontFamily: serif,
      fontSize: 20,
      lineHeight: 26,
      fontWeight: "700",
    } satisfies TextStyle,
    eyebrow: {
      fontFamily: sans,
      fontSize: 12,
      lineHeight: 16,
      fontWeight: "600",
      letterSpacing: 1.2,
      textTransform: "uppercase",
    } satisfies TextStyle,
    bodyLg: {
      fontFamily: sans,
      fontSize: 19,
      lineHeight: 26,
      fontWeight: "400",
    } satisfies TextStyle,
    body: {
      fontFamily: sans,
      fontSize: 17,
      lineHeight: 24,
      fontWeight: "400",
    } satisfies TextStyle,
    bodyEmphasis: {
      fontFamily: sans,
      fontSize: 17,
      lineHeight: 24,
      fontWeight: "600",
    } satisfies TextStyle,
    label: {
      fontFamily: sans,
      fontSize: 15,
      lineHeight: 20,
      fontWeight: "500",
      letterSpacing: 0.1,
    } satisfies TextStyle,
    caption: {
      fontFamily: sans,
      fontSize: 13,
      lineHeight: 18,
      fontWeight: "500",
    } satisfies TextStyle,
    mono: {
      fontFamily: mono,
      fontSize: 15,
      lineHeight: 20,
    } satisfies TextStyle,
  },
```

Note `label` is new to `theme.text` — it already existed in `tailwind.config.js` but not here. The two sources now match.

- [ ] **Step 4: Update `tailwind.config.js`**

Replace the `fontSize` block (currently `tailwind.config.js:68-78`) with:

```js
      fontSize: {
        hero: ['44px', { lineHeight: '48px', letterSpacing: '-0.8px' }],
        display: ['40px', { lineHeight: '46px', letterSpacing: '-0.6px' }],
        h1: ['30px', { lineHeight: '36px', letterSpacing: '-0.4px' }],
        h2: ['24px', { lineHeight: '30px', letterSpacing: '-0.25px' }],
        h3: ['20px', { lineHeight: '26px', letterSpacing: '0px' }],
        'body-lg': ['19px', { lineHeight: '26px', letterSpacing: '0px' }],
        body: ['17px', { lineHeight: '24px', letterSpacing: '0px' }],
        label: ['15px', { lineHeight: '20px', letterSpacing: '0.1px' }],
        caption: ['13px', { lineHeight: '18px', letterSpacing: '0.2px' }],
        eyebrow: ['12px', { lineHeight: '16px', letterSpacing: '1.2px' }],
      },
```

- [ ] **Step 5: Add the `heroNumeral` preset to `Text.tsx`**

In `apps/mobile-admin/components/ui/Text.tsx`, add `'heroNumeral'` as the first member of the `TextPreset` union (line 3-12):

```tsx
export type TextPreset =
  | 'heroNumeral'
  | 'display'
  | 'h1'
  | 'h2'
  | 'h3'
  | 'eyebrow'
  | 'bodyLg'
  | 'body'
  | 'bodyEmphasis'
  | 'label'
  | 'caption';
```

and add the two matching entries to `PRESET_CLASSES` (line 16-26) — `heroNumeral` first, `label` between `bodyEmphasis` and `caption`:

```tsx
  heroNumeral: 'font-serif-bold text-hero text-ink',
```

```tsx
  label: 'font-sans-medium text-label text-ink',
```

`label` already existed in `tailwind.config.js` but in neither `theme.text` nor `Text.tsx`. Adding it to all three keeps the sources in agreement, so `<Text preset="label">` works and `theme.text.label` is not a dead token.

- [ ] **Step 6: Run the tests to verify they pass**

```bash
cd apps/mobile-admin && npx jest __tests__/theme-tokens.test.ts components/ui/Text.test.tsx
```

Expected: PASS, all assertions green.

- [ ] **Step 7: Run the full gates**

```bash
cd apps/mobile-admin && npm test && npm run check-types
```

Expected: 352+ tests pass, `tsc` exits 0. If a snapshot-free layout test fails on a hardcoded size, fix the test to read the token rather than the literal — do not revert the token.

- [ ] **Step 8: Commit**

```bash
git add apps/mobile-admin/lib/theme.ts apps/mobile-admin/tailwind.config.js \
        apps/mobile-admin/components/ui/Text.tsx apps/mobile-admin/components/ui/Text.test.tsx \
        apps/mobile-admin/__tests__/theme-tokens.test.ts
git commit -m "feat(mobile-admin): re-scale type tokens to native metrics (body 14->17)"
```

---

### Task 2: Density and surface tokens

**Files:**
- Modify: `apps/mobile-admin/lib/theme.ts` (`colors` block and a new `row`/`thumb` block)
- Test: `apps/mobile-admin/__tests__/theme-tokens.test.ts` (extend)

**Interfaces:**
- Consumes: Task 1's `theme` shape.
- Produces:
  - `theme.colors.sink: "#ECEAE3"`
  - `theme.row: { minHeightSingle: 64; minHeightDouble: 88; paddingH: 20; paddingV: 14; gap: 16 }`
  - `theme.thumb: { list: 60; compact: 38 }`

  Tasks 3, 4, 6 and 7 read these instead of literals.

- [ ] **Step 1: Write the failing tests**

Append to `apps/mobile-admin/__tests__/theme-tokens.test.ts`:

```ts
describe('density tokens', () => {
  it('sets native row metrics', () => {
    expect(theme.row.minHeightSingle).toBe(64);
    expect(theme.row.minHeightDouble).toBe(88);
    expect(theme.row.paddingH).toBe(20);
    expect(theme.row.paddingV).toBe(14);
    expect(theme.row.gap).toBe(16);
  });

  it('sets thumbnail sizes', () => {
    expect(theme.thumb.list).toBe(60);
    expect(theme.thumb.compact).toBe(38);
  });

  it('keeps the 44pt minimum touch target', () => {
    expect(theme.touchTarget).toBe(44);
  });

  // Guardrail 8: theme.radii.md and tailwind rounded-md were deliberately
  // reconciled to 6px in the 2026-07-17 pass. They must not drift apart again.
  it('keeps radii.md at 6px', () => {
    expect(theme.radii.md).toBe(6);
    expect(theme.radius).toBe(6);
  });

  it('exposes the sink surface for iOS press feedback', () => {
    expect(theme.colors.sink).toBe('#ECEAE3');
  });
});
```

- [ ] **Step 2: Run to verify failure**

```bash
cd apps/mobile-admin && npx jest __tests__/theme-tokens.test.ts -t "density tokens"
```

Expected: FAIL — `Cannot read properties of undefined (reading 'minHeightSingle')`.

- [ ] **Step 3: Add the tokens**

In `apps/mobile-admin/lib/theme.ts`, add `sink` to the `palette` const (after `bone`):

```ts
  sink: "#ECEAE3",
```

add to `colors` (after `surfaceAlt`):

```ts
    sink: palette.sink,
```

and add a new block immediately after `radii` / before `hairline`:

```ts
  /**
   * Native row density. iOS list rows carry their content on a taller,
   * calmer field than a web table row — 64 for a single line, 88 for the
   * two-line stack (17pt primary + 13pt secondary). `touchTarget` still
   * holds at 44; rows exceed it rather than replacing the rule.
   */
  row: {
    minHeightSingle: 64,
    minHeightDouble: 88,
    paddingH: 20,
    paddingV: 14,
    gap: 16,
  },

  thumb: {
    list: 60,
    compact: 38,
  },
```

- [ ] **Step 4: Run to verify pass**

```bash
cd apps/mobile-admin && npx jest __tests__/theme-tokens.test.ts && npm run check-types
```

Expected: PASS, `tsc` exits 0.

- [ ] **Step 5: Commit**

```bash
git add apps/mobile-admin/lib/theme.ts apps/mobile-admin/__tests__/theme-tokens.test.ts
git commit -m "feat(mobile-admin): add native row density and sink surface tokens"
```

---

### Task 3: `PressableRow` primitive

**Files:**
- Create: `apps/mobile-admin/components/ui/PressableRow.tsx`
- Modify: `apps/mobile-admin/components/ui/index.ts`
- Test: `apps/mobile-admin/__tests__/pressable-row.test.tsx` (create)

**Interfaces:**
- Consumes: `theme.row`, `theme.colors.sink` from Task 2.
- Produces:

```ts
export interface PressableRowProps {
  children: ReactNode;
  onPress: () => void;
  onLongPress?: () => void;
  lines?: 1 | 2;              // default 1
  style?: StyleProp<ViewStyle>;
  accessibilityLabel: string;
  testID?: string;
}
export function PressableRow(props: PressableRowProps): JSX.Element;
```

  Tasks 6 and 7 wrap every list row in this. It owns press feedback and density only — never business logic.

- [ ] **Step 1: Write the failing test**

Create `apps/mobile-admin/__tests__/pressable-row.test.tsx`:

```tsx
import { Text as RNText, StyleSheet } from 'react-native';
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

  it('exposes an Android ripple at 12% ink', () => {
    const { getByTestId } = render(
      <PressableRow onPress={() => {}} accessibilityLabel="Row" testID="row">
        <RNText>Row</RNText>
      </PressableRow>,
    );
    expect(getByTestId('row').props.android_ripple).toEqual({
      color: 'rgba(14, 14, 12, 0.12)',
    });
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
```

- [ ] **Step 2: Run to verify failure**

```bash
cd apps/mobile-admin && npx jest __tests__/pressable-row.test.tsx
```

Expected: FAIL — `Cannot find module '@/components/ui/PressableRow'`.

- [ ] **Step 3: Implement `PressableRow`**

Create `apps/mobile-admin/components/ui/PressableRow.tsx`:

```tsx
import type { ReactNode } from "react";
import {
  Platform,
  Pressable,
  StyleSheet,
  type StyleProp,
  type ViewStyle,
} from "react-native";
import { theme } from "@/lib/theme";

/**
 * The one row press surface in the app.
 *
 * Replaces `TouchableOpacity` + `activeOpacity`: a whole-row 60% fade is a
 * web-styled-RN signature, not native press feedback. iOS shifts the row
 * background to the sink surface while held; Android draws a ripple.
 *
 * Owns press feedback and density ONLY. Callers supply all content and all
 * handlers — no business logic lives here.
 */
export interface PressableRowProps {
  children: ReactNode;
  onPress: () => void;
  onLongPress?: () => void;
  /** 1 for a single-line row (64pt), 2 for the primary+secondary stack (88pt). */
  lines?: 1 | 2;
  style?: StyleProp<ViewStyle>;
  accessibilityLabel: string;
  testID?: string;
}

const RIPPLE = { color: "rgba(14, 14, 12, 0.12)" } as const;

export function PressableRow({
  children,
  onPress,
  onLongPress,
  lines = 1,
  style,
  accessibilityLabel,
  testID,
}: PressableRowProps) {
  return (
    <Pressable
      onPress={onPress}
      onLongPress={onLongPress}
      accessibilityRole="button"
      accessibilityLabel={accessibilityLabel}
      testID={testID}
      android_ripple={RIPPLE}
      style={({ pressed }) => [
        styles.base,
        lines === 2 ? styles.twoLine : styles.oneLine,
        // Android draws its own ripple; only iOS needs the background shift.
        pressed && Platform.OS === "ios" ? styles.pressed : null,
        style,
      ]}
    >
      {children}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  base: {
    flexDirection: "row",
    alignItems: "center",
    gap: theme.row.gap,
    paddingHorizontal: theme.row.paddingH,
    paddingVertical: theme.row.paddingV,
    backgroundColor: theme.colors.background,
  },
  oneLine: { minHeight: theme.row.minHeightSingle },
  twoLine: { minHeight: theme.row.minHeightDouble },
  pressed: { backgroundColor: theme.colors.sink },
});
```

- [ ] **Step 4: Export it**

Add to `apps/mobile-admin/components/ui/index.ts`, alphabetically among the existing exports:

```ts
export { PressableRow } from "./PressableRow";
export type { PressableRowProps } from "./PressableRow";
```

- [ ] **Step 5: Run to verify pass**

```bash
cd apps/mobile-admin && npx jest __tests__/pressable-row.test.tsx && npm run check-types
```

Expected: 7 tests PASS, `tsc` exits 0.

- [ ] **Step 6: Commit**

```bash
git add apps/mobile-admin/components/ui/PressableRow.tsx \
        apps/mobile-admin/components/ui/index.ts \
        apps/mobile-admin/__tests__/pressable-row.test.tsx
git commit -m "feat(mobile-admin): add PressableRow with platform-native press feedback"
```

---

### Task 4: `Thumb` primitive on expo-image

**Files:**
- Create: `apps/mobile-admin/components/ui/Thumb.tsx`
- Modify: `apps/mobile-admin/components/ui/index.ts`
- Test: `apps/mobile-admin/__tests__/thumb.test.tsx` (create)

**Interfaces:**
- Consumes: `theme.thumb`, `theme.radii`, `theme.colors` from Tasks 1–2.
- Produces:

```ts
export interface ThumbProps {
  uri?: string | null;
  size?: "list" | "compact";   // default "list" (60pt); compact is 38pt
  recyclingKey?: string;
  accessibilityLabel?: string;
  testID?: string;
}
export function Thumb(props: ThumbProps): JSX.Element;
```

  Task 6 uses it in `ProductRow`; later increments use it for queue rows. A missing or failed `uri` always renders the placeholder at the same dimensions, so row height never shifts.

- [ ] **Step 1: Write the failing test**

Create `apps/mobile-admin/__tests__/thumb.test.tsx`:

```tsx
import { StyleSheet } from 'react-native';
import { render } from '@testing-library/react-native';

jest.mock('expo-image', () => {
  const { View } = require('react-native');
  return { Image: View };
});

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
```

- [ ] **Step 2: Run to verify failure**

```bash
cd apps/mobile-admin && npx jest __tests__/thumb.test.tsx
```

Expected: FAIL — `Cannot find module '@/components/ui/Thumb'`.

- [ ] **Step 3: Implement `Thumb`**

Create `apps/mobile-admin/components/ui/Thumb.tsx`:

```tsx
import { View, StyleSheet } from "react-native";
import { Image } from "expo-image";
import { Package } from "lucide-react-native";
import { theme } from "@/lib/theme";

/**
 * List thumbnail on expo-image (already a dependency, previously used only
 * in ProductMediaPicker). react-native's `Image` pops in with no transition,
 * which reads as a web image load.
 *
 * A missing, null, or failed `uri` renders the placeholder at the SAME
 * dimensions, so a row never changes height because an image 404'd.
 */
export interface ThumbProps {
  uri?: string | null;
  /** "list" = 60pt (default), "compact" = 38pt. */
  size?: "list" | "compact";
  /** Stable item id — stops FlatList recycling flashing a stale image. */
  recyclingKey?: string;
  accessibilityLabel?: string;
  testID?: string;
}

export function Thumb({
  uri,
  size = "list",
  recyclingKey,
  accessibilityLabel,
  testID,
}: ThumbProps) {
  const dim = size === "list" ? theme.thumb.list : theme.thumb.compact;
  const box = { width: dim, height: dim };

  if (!uri) {
    return (
      <View style={[styles.box, box, styles.placeholder]} testID={testID}>
        <Package
          size={Math.round(dim / 3)}
          color={theme.colors.textTertiary}
          strokeWidth={1.5}
        />
      </View>
    );
  }

  return (
    <Image
      source={{ uri }}
      style={[styles.box, box]}
      contentFit="cover"
      transition={200}
      recyclingKey={recyclingKey}
      accessibilityLabel={accessibilityLabel}
      testID={testID}
    />
  );
}

const styles = StyleSheet.create({
  box: {
    borderRadius: theme.radii.md,
    backgroundColor: theme.colors.sink,
    flexShrink: 0,
  },
  placeholder: {
    alignItems: "center",
    justifyContent: "center",
  },
});
```

- [ ] **Step 4: Export it**

Add to `apps/mobile-admin/components/ui/index.ts`:

```ts
export { Thumb } from "./Thumb";
export type { ThumbProps } from "./Thumb";
```

- [ ] **Step 5: Run to verify pass**

```bash
cd apps/mobile-admin && npx jest __tests__/thumb.test.tsx && npm run check-types
```

Expected: 5 tests PASS, `tsc` exits 0.

- [ ] **Step 6: Commit**

```bash
git add apps/mobile-admin/components/ui/Thumb.tsx \
        apps/mobile-admin/components/ui/index.ts \
        apps/mobile-admin/__tests__/thumb.test.tsx
git commit -m "feat(mobile-admin): add Thumb primitive on expo-image with stable placeholder"
```

---

### Task 5: Admin haptics triggers

**Files:**
- Modify: `packages/mobile-shared/haptics/feedback.ts`
- Test: `packages/mobile-shared/haptics/__tests__/admin-feedback.test.ts` (create)

**Interfaces:**
- Consumes: nothing.
- Produces:

```ts
export const adminHaptics: {
  selectionChanged: () => Promise<void>;
  swipeThreshold: () => Promise<void>;
  menuOpen: () => Promise<void>;
  actionSucceeded: () => Promise<void>;
  actionFailed: () => Promise<void>;
};
```

  Imported as `import { adminHaptics } from "@repo/mobile-shared/haptics/feedback"`. Every trigger swallows platform errors and never rejects. Increment 2 wires these to tab change, swipe threshold, sheet open, and mutation outcomes.

The existing `haptics` export (storefront-shaped: `addToCart`, `checkoutStep`, …) is left untouched — the storefront app depends on it.

- [ ] **Step 1: Write the failing test**

Create `packages/mobile-shared/haptics/__tests__/admin-feedback.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from "vitest";

const {
  mockImpactAsync,
  mockNotificationAsync,
  mockSelectionAsync,
} = vi.hoisted(() => ({
  mockImpactAsync: vi.fn().mockResolvedValue(undefined),
  mockNotificationAsync: vi.fn().mockResolvedValue(undefined),
  mockSelectionAsync: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("expo-haptics", () => ({
  impactAsync: mockImpactAsync,
  notificationAsync: mockNotificationAsync,
  selectionAsync: mockSelectionAsync,
  ImpactFeedbackStyle: { Light: "Light", Medium: "Medium", Heavy: "Heavy" },
  NotificationFeedbackType: {
    Success: "Success",
    Warning: "Warning",
    Error: "Error",
  },
}));

import { adminHaptics } from "../feedback";
import * as Haptics from "expo-haptics";

describe("adminHaptics", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockImpactAsync.mockResolvedValue(undefined);
    mockNotificationAsync.mockResolvedValue(undefined);
    mockSelectionAsync.mockResolvedValue(undefined);
  });

  it("selectionChanged calls selectionAsync", async () => {
    await adminHaptics.selectionChanged();
    expect(mockSelectionAsync).toHaveBeenCalledOnce();
  });

  it("swipeThreshold calls impactAsync with Light", async () => {
    await adminHaptics.swipeThreshold();
    expect(mockImpactAsync).toHaveBeenCalledWith(
      Haptics.ImpactFeedbackStyle.Light,
    );
  });

  it("menuOpen calls impactAsync with Medium", async () => {
    await adminHaptics.menuOpen();
    expect(mockImpactAsync).toHaveBeenCalledWith(
      Haptics.ImpactFeedbackStyle.Medium,
    );
  });

  it("actionSucceeded calls notificationAsync with Success", async () => {
    await adminHaptics.actionSucceeded();
    expect(mockNotificationAsync).toHaveBeenCalledWith(
      Haptics.NotificationFeedbackType.Success,
    );
  });

  it("actionFailed calls notificationAsync with Error", async () => {
    await adminHaptics.actionFailed();
    expect(mockNotificationAsync).toHaveBeenCalledWith(
      Haptics.NotificationFeedbackType.Error,
    );
  });

  it("never rejects when the platform has no haptics engine", async () => {
    mockSelectionAsync.mockRejectedValueOnce(new Error("Unsupported"));
    await expect(adminHaptics.selectionChanged()).resolves.toBeUndefined();
  });

  it("never rejects when an impact call throws synchronously", async () => {
    mockImpactAsync.mockImplementationOnce(() => {
      throw new Error("Unsupported");
    });
    await expect(adminHaptics.swipeThreshold()).resolves.toBeUndefined();
  });
});
```

- [ ] **Step 2: Run to verify failure**

```bash
cd packages/mobile-shared && npx vitest run haptics/__tests__/admin-feedback.test.ts
```

Expected: FAIL — `adminHaptics` is not exported from `../feedback`.

- [ ] **Step 3: Implement `adminHaptics`**

Append to `packages/mobile-shared/haptics/feedback.ts` (leave the existing `haptics` export exactly as it is):

```ts
/**
 * Haptics are unavailable on simulators and on some Android hardware, where
 * expo-haptics either rejects or throws synchronously. Feedback is never
 * worth failing an interaction over, so every admin trigger is fire-and-forget.
 */
function safe(fn: () => Promise<void> | void): () => Promise<void> {
  return async () => {
    try {
      await fn();
    } catch {
      // Intentionally swallowed — haptic feedback is never user-critical and
      // must never surface as an unhandled rejection.
    }
  };
}

/**
 * Admin-side feedback vocabulary. Named for the moment, not the waveform, so
 * call sites read as intent. Separate from `haptics` above, which is
 * storefront-shaped (addToCart / checkoutStep / …) and used by another app.
 */
export const adminHaptics = {
  /** Tab change, filter chip, segmented control. */
  selectionChanged: safe(() => Haptics.selectionAsync()),
  /** A swipe gesture crosses its action threshold. Fires once per crossing. */
  swipeThreshold: safe(() =>
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light),
  ),
  /** A long-press opens an action sheet. */
  menuOpen: safe(() =>
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium),
  ),
  /** Order fulfilled, review approved, save succeeded. */
  actionSucceeded: safe(() =>
    Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success),
  ),
  /** Action failed or validation blocked. */
  actionFailed: safe(() =>
    Haptics.notificationAsync(Haptics.NotificationFeedbackType.Error),
  ),
} as const;
```

- [ ] **Step 4: Run to verify pass**

```bash
cd packages/mobile-shared && npx vitest run haptics/__tests__/admin-feedback.test.ts
```

Expected: 7 tests PASS.

- [ ] **Step 5: Run the full mobile-shared gates**

```bash
cd packages/mobile-shared && npm test && npm run check-types
```

Expected: 90 tests pass (83 baseline + 7 new), `tsc` exits 0.

- [ ] **Step 6: Commit**

```bash
git add packages/mobile-shared/haptics/feedback.ts \
        packages/mobile-shared/haptics/__tests__/admin-feedback.test.ts
git commit -m "feat(mobile-shared): add adminHaptics trigger set with safe fire-and-forget calls"
```

---

### Task 6: Migrate the list row components

**Files:**
- Modify: `apps/mobile-admin/components/ProductRow.tsx`
- Modify: `apps/mobile-admin/components/OrderRow.tsx`
- Modify: `apps/mobile-admin/components/CustomerRow.tsx`
- Modify: `apps/mobile-admin/components/dashboard/DashboardOrderRow.tsx`
- Modify: `apps/mobile-admin/components/reviews/ReviewRow.tsx`
- Modify: `apps/mobile-admin/components/marketing/CampaignRow.tsx`
- Modify: `apps/mobile-admin/components/marketing/CouponRow.tsx`
- Modify: `apps/mobile-admin/components/marketing/GiftCardRow.tsx`
- Modify: `apps/mobile-admin/components/marketing/MarketingRow.tsx`
- Modify: `apps/mobile-admin/components/marketing/SegmentRow.tsx`
- Test: existing `__tests__/customer-row.test.tsx` and `__tests__/reviews.test.tsx` must stay green

**Interfaces:**
- Consumes: `PressableRow` (Task 3), `Thumb` (Task 4), `theme.row` (Task 2).
- Produces: no new exports. Every row component keeps its existing public props exactly — only its internals change. This matters: Task 7 and the later increments call these unchanged.

- [ ] **Step 1: Write the failing test**

Create `apps/mobile-admin/__tests__/row-density.test.tsx`:

```tsx
import { StyleSheet } from 'react-native';
import { render } from '@testing-library/react-native';

jest.mock('expo-image', () => {
  const { View } = require('react-native');
  return { Image: View };
});

import { ProductRow } from '@/components/ProductRow';
import { theme } from '@/lib/theme';
import type { Product } from '@repo/mobile-shared/api/types';

const product = {
  id: 'p1',
  title: 'Bondi Linen Shirt',
  status: 'active',
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
```

- [ ] **Step 2: Run to verify failure**

```bash
cd apps/mobile-admin && npx jest __tests__/row-density.test.tsx
```

Expected: FAIL — no element with testID `product-row-p1`.

- [ ] **Step 3: Migrate `ProductRow` (the reference transformation)**

Replace the whole of `apps/mobile-admin/components/ProductRow.tsx` with:

```tsx
import { View, StyleSheet } from "react-native";
import { PressableRow, StatusBadge, Text, Thumb } from "@/components/ui";
import { theme } from "@/lib/theme";
import type { Product } from "@repo/mobile-shared/api/types";
import {
  formatMoney,
  productCurrency,
  productPrice,
  productStock,
  productThumb,
} from "@/lib/product-display";

interface ProductRowProps {
  product: Product;
  onPress: (product: Product) => void;
}

export function ProductRow({ product, onPress }: ProductRowProps) {
  const isActive = product.status === "active";
  const price = productPrice(product);
  const stock = productStock(product);
  const thumb = productThumb(product);
  const priceLabel =
    price === undefined ? "—" : formatMoney(price, productCurrency(product));
  const lowStock = stock <= 5;

  return (
    <PressableRow
      lines={2}
      onPress={() => onPress(product)}
      style={styles.row}
      testID={`product-row-${product.id}`}
      accessibilityLabel={`${product.title}, ${priceLabel}, stock ${stock}, ${product.status}`}
    >
      <Thumb
        uri={thumb}
        recyclingKey={product.id}
        accessibilityLabel={`${product.title} thumbnail`}
      />

      <View style={styles.info}>
        <Text preset="bodyEmphasis" color="text" numberOfLines={1}>
          {product.title}
        </Text>
        <View style={styles.metaRow}>
          <Text preset="caption" color="text">
            {priceLabel}
          </Text>
          <Text preset="caption" color={lowStock ? "danger" : "textTertiary"}>
            {stock} in stock
          </Text>
        </View>
      </View>

      <StatusBadge
        label={isActive ? "Active" : product.status}
        tone={isActive ? "success" : "muted"}
      />
    </PressableRow>
  );
}

const styles = StyleSheet.create({
  row: {
    backgroundColor: theme.colors.elevated,
    borderBottomWidth: theme.hairline,
    borderBottomColor: theme.colors.hairline,
  },
  info: { flex: 1, gap: 4, minWidth: 0 },
  metaRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: theme.spacing.md,
  },
});
```

What changed, and why — apply the same four moves to every other row in this task:

1. `TouchableOpacity` + `activeOpacity={0.6}` → `PressableRow`. Delete `activeOpacity` entirely.
2. `accessibilityRole="button"` and the container's flex/padding/gap/minHeight styles are now owned by `PressableRow` — remove them from the local `StyleSheet`. Keep only what is genuinely local (background, borders).
3. `Image` from react-native → `Thumb` with `recyclingKey` set to the item id. Delete the local `thumb`/`thumbPlaceholder` styles and the now-unused `Package` import.
4. Add `testID={`<name>-row-${item.id}`}` so the row is addressable in tests.

- [ ] **Step 4: Run the ProductRow test to verify it passes**

```bash
cd apps/mobile-admin && npx jest __tests__/row-density.test.tsx
```

Expected: 2 tests PASS.

- [ ] **Step 5: Commit the reference migration**

```bash
git add apps/mobile-admin/components/ProductRow.tsx apps/mobile-admin/__tests__/row-density.test.tsx
git commit -m "refactor(mobile-admin): migrate ProductRow to PressableRow and Thumb"
```

- [ ] **Step 6: Migrate `OrderRow` (the stacked-row case)**

`OrderRow` stacks three lines rather than sitting in a horizontal row, so it needs a
direction override. `PressableRow` applies the `style` prop last, so it wins over the base
`flexDirection: "row"`. Replace the whole of `apps/mobile-admin/components/OrderRow.tsx` with:

```tsx
import { View, StyleSheet } from "react-native";
import { PressableRow, Text } from "@/components/ui";
import { theme } from "@/lib/theme";
import { formatMoney } from "@/lib/money";
import { OrderStatusBadges } from "@/components/orders/OrderStatusBadges";
import type { Order } from "@repo/mobile-shared/api/types";

interface OrderRowProps {
  order: Order;
  onPress: (order: Order) => void;
  /** The active store's currency code (e.g. "AUD"). */
  currencyCode?: string;
}

function formatRelativeTime(dateString: string): string {
  const now = Date.now();
  const date = new Date(dateString).getTime();
  const diffMin = Math.floor((now - date) / 60_000);
  const diffHr = Math.floor((now - date) / 3_600_000);
  const diffDay = Math.floor((now - date) / 86_400_000);
  if (diffMin < 1) return "just now";
  if (diffMin < 60) return `${diffMin}m ago`;
  if (diffHr < 24) return `${diffHr}h ago`;
  if (diffDay < 30) return `${diffDay}d ago`;
  return new Date(dateString).toLocaleDateString("en-AU");
}

export function OrderRow({ order, onPress, currencyCode }: OrderRowProps) {
  const currency = order.currency_code || currencyCode;
  const displayName = order.customer_name || order.customer_email;
  const total = formatMoney(order.grand_total, currency);

  return (
    <PressableRow
      lines={2}
      onPress={() => onPress(order)}
      style={styles.row}
      testID={`order-row-${order.id}`}
      accessibilityLabel={`Order ${order.order_number}, ${displayName}, ${total}, ${order.status}`}
    >
      <View style={styles.stack}>
        <View style={styles.topRow}>
          <Text preset="bodyEmphasis" color="text">
            #{order.order_number}
          </Text>
          <OrderStatusBadges
            status={order.status}
            paymentStatus={order.payment_status}
          />
        </View>
        <Text
          preset="caption"
          color="textSecondary"
          numberOfLines={1}
          style={styles.customer}
        >
          {displayName}
        </Text>
        <View style={styles.bottomRow}>
          <Text preset="bodyEmphasis" color="text">
            {total}
          </Text>
          <Text preset="caption" color="textTertiary">
            {formatRelativeTime(order.placed_at)}
          </Text>
        </View>
      </View>
    </PressableRow>
  );
}

const styles = StyleSheet.create({
  row: {
    // Overrides PressableRow's base flexDirection: "row" — applied last, so
    // it wins. The three lines stack instead of sitting side by side.
    flexDirection: "column",
    alignItems: "stretch",
    backgroundColor: theme.colors.elevated,
    borderBottomWidth: theme.hairline,
    borderBottomColor: theme.colors.hairline,
  },
  stack: { gap: theme.spacing.xs },
  topRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    gap: theme.spacing.sm,
  },
  customer: { marginTop: 2 },
  bottomRow: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    marginTop: 2,
  },
});
```

- [ ] **Step 7: Apply the same four moves to the remaining eight row components**

`CustomerRow`, `DashboardOrderRow`, `ReviewRow`, `CampaignRow`, `CouponRow`, `GiftCardRow`,
`MarketingRow`, `SegmentRow`.

Pick `lines` and direction per row using the two worked examples above as the reference:

| Component | `lines` | Direction | Notes |
|---|---|---|---|
| `CustomerRow` | 2 | row (default) | Avatar stays neutral — never moss (guardrail 2) |
| `DashboardOrderRow` | 1 | row (default) | Single-line — no override |
| `ReviewRow` | 2 | column (like `OrderRow`) | Stars, then quote, then meta |
| `CampaignRow` | 2 | row (default) | |
| `CouponRow` | 2 | row (default) | |
| `GiftCardRow` | 2 | row (default) | |
| `MarketingRow` | 1 | row (default) | Menu row, single line |
| `SegmentRow` | 2 | row (default) | |

Every one of them: delete `activeOpacity`, delete `accessibilityRole="button"` (PressableRow
sets it), delete the container's `flexDirection`/`alignItems`/`paddingHorizontal`/
`paddingVertical`/`gap`/`minHeight` from its local `StyleSheet`, and add
`testID={`<name>-row-${item.id}`}`.

- [ ] **Step 8: Run the full app gates**

```bash
cd apps/mobile-admin && npm test && npm run check-types
```

Expected: all tests pass (`customer-row.test.tsx` and `reviews.test.tsx` in particular), `tsc` exits 0. If a test asserted a literal `paddingVertical: 12`, update it to read `theme.row.paddingV` — do not revert the density.

- [ ] **Step 9: Commit**

```bash
git add apps/mobile-admin/components
git commit -m "refactor(mobile-admin): migrate all list rows to PressableRow and native density"
```

---

### Task 7: Migrate the remaining `TouchableOpacity` call sites

**Files (31 remaining after Task 6):**

UI primitives — `components/ui/BackHeader.tsx`, `components/ui/SearchField.tsx`, `components/ui/SegmentedControl.tsx`

Components — `components/StorePicker.tsx`, `components/StoreSelector.tsx`, `components/TenantGate.tsx`, `components/dashboard/NotificationBell.tsx`, `components/ProductMediaPicker.tsx`, `components/products/CreateNextStepsBanner.tsx`, `components/products/ImageViewer.tsx`

Screens — `app/notifications.tsx`, `app/(tabs)/index.tsx`, `app/(tabs)/customers/[id].tsx`, `app/(tabs)/customers/reviews/[id].tsx`, `app/(tabs)/orders/[id].tsx`, `app/(tabs)/products/index.tsx`, `app/(tabs)/products/[id].tsx`, `app/(tabs)/products/new.tsx`, `app/(tabs)/more/index.tsx`, `app/(tabs)/more/account.tsx`, `app/(tabs)/more/security.tsx`, `app/(tabs)/more/settings/team/index.tsx`, `app/(tabs)/more/settings/tickets/index.tsx`, `app/(tabs)/more/marketing/campaigns/index.tsx`, `app/(tabs)/more/marketing/campaigns/[id].tsx`, `app/(tabs)/more/marketing/coupons/index.tsx`, `app/(tabs)/more/marketing/coupons/[id].tsx`, `app/(tabs)/more/marketing/gift-cards/index.tsx`, `app/(tabs)/more/marketing/loyalty/members/index.tsx`, `app/(tabs)/more/marketing/segments/index.tsx`, `app/(tabs)/more/marketing/segments/[id].tsx`

**Interfaces:**
- Consumes: `PressableRow` (Task 3).
- Produces: no new exports. After this task, `activeOpacity` appears nowhere in the codebase.

Two distinct transformations — pick by what the element is:

**(a) A list row** → `PressableRow`, exactly as in Task 6. This covers `app/(tabs)/index.tsx`'s local `ListRow`, the `more/` menu rows, and the marketing index rows.

**(b) Anything else** (icon buttons, links, chips, banner dismissals, image tiles) → plain `Pressable`. Do **not** wrap these in `PressableRow`; it carries row padding they must not inherit.

For (b), the transformation is:

```tsx
// BEFORE
<TouchableOpacity onPress={onPress} activeOpacity={0.6} accessibilityRole="button" accessibilityLabel="Notifications">
  <Bell size={22} color={theme.colors.text} />
</TouchableOpacity>

// AFTER
<Pressable
  onPress={onPress}
  accessibilityRole="button"
  accessibilityLabel="Notifications"
  hitSlop={8}
  android_ripple={{ color: "rgba(14, 14, 12, 0.12)", borderless: true }}
  style={({ pressed }) => [pressed && Platform.OS === "ios" ? { opacity: 0.55 } : null]}
>
  <Bell size={22} color={theme.colors.text} />
</Pressable>
```

Icon buttons are the one place a brief opacity change is still correct — there is no background to shift on a transparent 24 pt glyph. Rows are not; rows get the sink background.

- [ ] **Step 1: Write the failing guard test**

Create `apps/mobile-admin/__tests__/no-touchable-opacity.test.ts`:

```ts
import { execSync } from 'child_process';
import path from 'path';

const APP_ROOT = path.resolve(__dirname, '..');

function grepCount(pattern: string): number {
  try {
    const out = execSync(
      `grep -rl "${pattern}" app components lib 2>/dev/null || true`,
      { cwd: APP_ROOT, encoding: 'utf8' },
    );
    return out.split('\n').filter(Boolean).length;
  } catch {
    return 0;
  }
}

describe('press feedback migration', () => {
  it('has no remaining TouchableOpacity imports', () => {
    expect(grepCount('TouchableOpacity')).toBe(0);
  });

  it('has no remaining activeOpacity props', () => {
    expect(grepCount('activeOpacity')).toBe(0);
  });
});
```

- [ ] **Step 2: Run to verify failure**

```bash
cd apps/mobile-admin && npx jest __tests__/no-touchable-opacity.test.ts
```

Expected: FAIL — reports 31 files still containing `TouchableOpacity`.

- [ ] **Step 3: Migrate the three UI primitives first**

`BackHeader`, `SearchField`, `SegmentedControl` are used by many screens, so they go first and their existing tests catch regressions immediately. All three are transformation (b). `SegmentedControl` and `SearchField` already set `minHeight: theme.touchTarget` (44) — keep that, it is unchanged.

While in `SegmentedControl`, wire the selection haptic:

```tsx
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";

// inside the segment's onPress, before onChange:
void adminHaptics.selectionChanged();
onChange(segment.key);
```

- [ ] **Step 4: Run the primitives' gates**

```bash
cd apps/mobile-admin && npm test && npm run check-types
```

Expected: all pass. Commit before continuing.

```bash
git add apps/mobile-admin/components/ui
git commit -m "refactor(mobile-admin): migrate ui primitives to Pressable with selection haptic"
```

- [ ] **Step 5: Migrate the seven remaining components**

`StorePicker`, `StoreSelector`, `TenantGate`, `NotificationBell`, `ProductMediaPicker`, `CreateNextStepsBanner`, `ImageViewer` — all transformation (b) except `StorePicker`/`StoreSelector` store rows, which are (a).

- [ ] **Step 6: Migrate the 21 screens**

Work top-down through the list above. In `app/(tabs)/index.tsx`, the local `ListRow` component becomes a `PressableRow` and its `styles.row` block (`app/(tabs)/index.tsx:351-358`) is deleted — `PressableRow` now owns `flexDirection`, `alignItems`, `paddingHorizontal`, `paddingVertical`, `gap`, and `minHeight`.

- [ ] **Step 7: Run the guard test and the full gates**

```bash
cd apps/mobile-admin && npx jest __tests__/no-touchable-opacity.test.ts && npm test && npm run check-types
```

Expected: guard reports 0 files for both patterns; all tests pass; `tsc` exits 0.

- [ ] **Step 8: Commit**

```bash
git add apps/mobile-admin
git commit -m "refactor(mobile-admin): replace every TouchableOpacity with native Pressable feedback"
```

---

### Task 8: Contrast regression guard

**Files:**
- Modify: `apps/mobile-admin/__tests__/theme-tokens.test.ts`

**Interfaces:**
- Consumes: `lib/theme.ts` and `tailwind.config.js` as files on disk.
- Produces: no exports. A permanent gate that fails CI if either banned colour returns.

- [ ] **Step 1: Write the failing test**

Append to `apps/mobile-admin/__tests__/theme-tokens.test.ts`:

```ts
import fs from 'fs';
import path from 'path';

describe('WCAG AA colour guard', () => {
  // Both of these fail 4.5:1 against Paper #F7F6F2 and were removed in the
  // 2026-07-17 design pass. See docs/superpowers/design-scan/.
  const BANNED = [
    'rgba(14, 14, 12, 0.5)',
    'rgba(14,14,12,0.5)',
    '#7A766E',
    '#7a766e',
  ];

  const sources = [
    path.resolve(__dirname, '../lib/theme.ts'),
    path.resolve(__dirname, '../tailwind.config.js'),
  ];

  for (const file of sources) {
    it(`does not reintroduce a failing text colour in ${path.basename(file)}`, () => {
      const contents = fs.readFileSync(file, 'utf8');
      for (const banned of BANNED) {
        expect(contents).not.toContain(banned);
      }
    });
  }

  it('keeps tertiary text at the AA-passing value in both sources', () => {
    expect(theme.colors.textTertiary).toBe('#5C5953');
    const tw = fs.readFileSync(
      path.resolve(__dirname, '../tailwind.config.js'),
      'utf8',
    );
    expect(tw).toContain('#5C5953');
  });
});
```

- [ ] **Step 2: Run it**

```bash
cd apps/mobile-admin && npx jest __tests__/theme-tokens.test.ts -t "WCAG AA colour guard"
```

Expected: PASS immediately — the banned values are already absent. This test is a ratchet, not a fix. If it fails, a banned colour was reintroduced during Tasks 1–7 and must be corrected before continuing.

- [ ] **Step 3: Commit**

```bash
git add apps/mobile-admin/__tests__/theme-tokens.test.ts
git commit -m "test(mobile-admin): guard against reintroducing AA-failing text colours"
```

---

### Task 9: marketplace-api dashboard fields

**Files:**
- Modify: `services/marketplace-api/internal/handlers/admin/dashboard.go` — DTOs at lines 37-62, queries at lines 235-322

**Interfaces:**
- Consumes: nothing.
- Produces: `GET /admin/stores/:storeId/dashboard` gains four JSON fields:
  - `recent_orders[].customer_name` — `*string`, omitempty
  - `recent_orders[].image_url` — `*string`, omitempty
  - `low_stock[].product_id` — `string`, always present
  - `low_stock[].image_url` — `*string`, omitempty

  Task 10 widens the zod schema to accept them.

**Why `product_id` is here.** `low_stock[].id` is `pv.id` — the *variant* id, taken from `FROM product_variants pv`. The dashboard's low-stock row already navigates with `router.push('/(tabs)/products/${item.id}')`, which pushes a variant id into a product route. That is a pre-existing bug, and increment 2's queue row depends on this navigation being correct, so it is fixed here rather than worked around.

`order_items.image_url` already exists as a column (`internal/order/models.go:64`), so the order thumbnail needs no join to `product_media`.

- [ ] **Step 1: Update the DTOs**

In `services/marketplace-api/internal/handlers/admin/dashboard.go`, replace the `RecentOrder` struct (lines 37-45):

```go
// RecentOrder is a summary row for a recent order.
type RecentOrder struct {
	ID            string  `json:"id"`
	OrderNumber   string  `json:"order_number"`
	CustomerEmail string  `json:"customer_email"`
	// Nullable in the DB — merchants can place orders without a name.
	// Mobile falls back to CustomerEmail when absent.
	CustomerName *string `json:"customer_name,omitempty"`
	GrandTotal   float64 `json:"grand_total"`
	Status       string  `json:"status"`
	CreatedAt    string  `json:"created_at"`
	// First line item's product image, for the mobile queue thumbnail.
	// Absent when the order has no items with an image (e.g. the product
	// was deleted after the order was placed).
	ImageURL *string `json:"image_url,omitempty"`
}
```

and replace the `LowStockItem` struct (lines 56-62):

```go
// LowStockItem shows a variant below its reorder threshold.
type LowStockItem struct {
	// ID is the VARIANT id. Use ProductID to navigate to the product.
	ID                string  `json:"id"`
	ProductID         string  `json:"product_id"`
	Title             string  `json:"title"`
	VariantTitle      string  `json:"variant_title"`
	Quantity          int     `json:"quantity"`
	LowStockThreshold int     `json:"low_stock_threshold"`
	ImageURL          *string `json:"image_url,omitempty"`
}
```

- [ ] **Step 2: Update the recent-orders query**

Replace the recent-orders block (`dashboard.go:235-261`) with:

```go
	// Recent orders — last 5.
	var recentRows []struct {
		ID            uuid.UUID
		OrderNumber   string
		CustomerEmail string
		CustomerName  *string
		GrandTotal    float64
		Status        string
		CreatedAt     time.Time
		ImageURL      *string
	}
	db.Raw(`SELECT o.id, o.order_number, o.customer_email, o.customer_name,
			o.grand_total, o.status, o.created_at,
			(SELECT oi.image_url FROM order_items oi
			   WHERE oi.order_id = o.id AND oi.image_url IS NOT NULL
			   ORDER BY oi.created_at
			   LIMIT 1) AS image_url
		FROM orders o
		WHERE o.store_id = ? AND o.tenant_id = ?
		ORDER BY o.created_at DESC
		LIMIT 5`,
		storeID, tenantID).Scan(&recentRows)

	recentOrders := make([]RecentOrder, 0, len(recentRows))
	for _, r := range recentRows {
		recentOrders = append(recentOrders, RecentOrder{
			ID:            r.ID.String(),
			OrderNumber:   r.OrderNumber,
			CustomerEmail: r.CustomerEmail,
			CustomerName:  r.CustomerName,
			GrandTotal:    r.GrandTotal,
			Status:        r.Status,
			CreatedAt:     r.CreatedAt.Format("2006-01-02T15:04:05Z"),
			ImageURL:      r.ImageURL,
		})
	}
```

- [ ] **Step 3: Update the low-stock query**

Replace the low-stock block (`dashboard.go:294-322`) with:

```go
	// Low stock items.
	var lowRows []struct {
		ID                uuid.UUID
		ProductID         uuid.UUID
		Title             string
		VariantTitle      string
		Quantity          int
		LowStockThreshold int
		ImageURL          *string
	}
	db.Raw(`SELECT pv.id, pv.product_id, p.title, pv.title AS variant_title,
			pv.inventory_quantity AS quantity,
			COALESCE(pv.low_stock_threshold, 10) AS low_stock_threshold,
			(SELECT pm.url FROM product_media pm
			   WHERE pm.product_id = p.id
			   ORDER BY pm.position
			   LIMIT 1) AS image_url
		FROM product_variants pv
		JOIN products p ON p.id = pv.product_id
		WHERE p.store_id = ? AND p.tenant_id = ?
		AND pv.inventory_quantity <= COALESCE(pv.low_stock_threshold, 10)
		AND pv.inventory_quantity > 0
		ORDER BY pv.inventory_quantity ASC
		LIMIT 10`,
		storeID, tenantID).Scan(&lowRows)

	lowStock := make([]LowStockItem, 0, len(lowRows))
	for _, r := range lowRows {
		lowStock = append(lowStock, LowStockItem{
			ID:                r.ID.String(),
			ProductID:         r.ProductID.String(),
			Title:             r.Title,
			VariantTitle:      r.VariantTitle,
			Quantity:          r.Quantity,
			LowStockThreshold: r.LowStockThreshold,
			ImageURL:          r.ImageURL,
		})
	}
```

The `image_url` subselect mirrors the one already used by the top-products query (`dashboard.go:271`), so the `product_media` ordering convention is unchanged.

- [ ] **Step 4: Build and vet**

```bash
cd services/marketplace-api && go build ./... && go vet ./... && go test ./internal/handlers/admin/...
```

Expected: build succeeds, vet is clean, existing handler tests pass.

- [ ] **Step 5: Commit**

```bash
git add services/marketplace-api/internal/handlers/admin/dashboard.go
git commit -m "feat(marketplace-api): add customer_name and image_url to dashboard rows, fix low_stock product_id"
```

---

### Task 10: Widen the dashboard schema and add client fallbacks

**Files:**
- Modify: `packages/mobile-shared/api/schemas/dashboard.ts`
- Modify: `apps/mobile-admin/app/(tabs)/index.tsx` (the `LowStockRow` navigation target)
- Test: `packages/mobile-shared/api/schemas/__tests__/dashboard.test.ts` (create)

**Interfaces:**
- Consumes: Task 9's wire shape.
- Produces: `RecentOrder` gains `customer_name?: string` and `image_url?: string`; `LowStockItem` gains `product_id?: string` and `image_url?: string`. All four are `.optional()` — a Go `omitempty` pointer is ABSENT from the JSON, never null. `product_id` is optional on the client even though Go always sends it, so a client build can ship before the API deploy.

- [ ] **Step 1: Write the failing test**

Create `packages/mobile-shared/api/schemas/__tests__/dashboard.test.ts`:

```ts
import { describe, it, expect } from "vitest";
import {
  recentOrderSchema,
  lowStockItemSchema,
} from "../dashboard";

const baseOrder = {
  id: "o1",
  order_number: "1042",
  customer_email: "ana@bondi.co",
  grand_total: 189,
  status: "pending",
  created_at: "2026-07-27T09:00:00Z",
};

const baseLowStock = {
  id: "v1",
  title: "Bondi Linen Shirt",
  variant_title: "M",
  quantity: 2,
  low_stock_threshold: 10,
};

describe("dashboard schema — new optional fields", () => {
  it("accepts a recent order without the new fields (pre-deploy API)", () => {
    const parsed = recentOrderSchema.parse(baseOrder);
    expect(parsed.customer_name).toBeUndefined();
    expect(parsed.image_url).toBeUndefined();
  });

  it("accepts a recent order with the new fields (post-deploy API)", () => {
    const parsed = recentOrderSchema.parse({
      ...baseOrder,
      customer_name: "Ana Ruiz",
      image_url: "https://cdn.example/shirt.jpg",
    });
    expect(parsed.customer_name).toBe("Ana Ruiz");
    expect(parsed.image_url).toBe("https://cdn.example/shirt.jpg");
  });

  it("rejects a null customer_name — omitempty means absent, not null", () => {
    expect(() =>
      recentOrderSchema.parse({ ...baseOrder, customer_name: null }),
    ).toThrow();
  });

  it("accepts a low-stock item without the new fields", () => {
    const parsed = lowStockItemSchema.parse(baseLowStock);
    expect(parsed.product_id).toBeUndefined();
    expect(parsed.image_url).toBeUndefined();
  });

  it("accepts a low-stock item with product_id and image_url", () => {
    const parsed = lowStockItemSchema.parse({
      ...baseLowStock,
      product_id: "p1",
      image_url: "https://cdn.example/shirt.jpg",
    });
    expect(parsed.product_id).toBe("p1");
    expect(parsed.image_url).toBe("https://cdn.example/shirt.jpg");
  });
});
```

- [ ] **Step 2: Run to verify failure**

```bash
cd packages/mobile-shared && npx vitest run api/schemas/__tests__/dashboard.test.ts
```

Expected: FAIL — the "with the new fields" cases pass through (zod objects are non-strict) but `parsed.customer_name` is `undefined` because the key is not in the schema, and the null-rejection case does not throw.

- [ ] **Step 3: Widen the schema**

In `packages/mobile-shared/api/schemas/dashboard.ts`, replace `recentOrderSchema`:

```ts
export const recentOrderSchema = z.object({
  id: z.string(),
  order_number: z.string(),
  customer_email: z.string(),
  // *string + omitempty -> ABSENT, not null. Falls back to customer_email.
  customer_name: z.string().optional(),
  grand_total: money,
  status: z.string(),
  created_at: z.string(),
  // First line item's product image. Absent when the order has no imaged
  // items — e.g. the product was deleted after the order was placed.
  image_url: z.string().optional(),
});
```

and replace `lowStockItemSchema`:

```ts
export const lowStockItemSchema = z.object({
  // `id` is the VARIANT id — navigate with product_id, not this.
  id: z.string(),
  // Optional so a client build can ship before the API deploy.
  product_id: z.string().optional(),
  title: z.string(),
  variant_title: z.string(),
  quantity: z.number(),
  low_stock_threshold: z.number(),
  image_url: z.string().optional(),
});
```

- [ ] **Step 4: Run to verify pass**

```bash
cd packages/mobile-shared && npx vitest run api/schemas/__tests__/dashboard.test.ts
```

Expected: 5 tests PASS.

- [ ] **Step 5: Fix the low-stock navigation target**

In `apps/mobile-admin/app/(tabs)/index.tsx`, the `LowStockRow` call site currently pushes `item.id` (a variant id). Change it to prefer `product_id` and skip navigation when neither the new field nor a usable id is present:

```tsx
            {data.low_stock.length > 0 ? (
              <Section title="Low Stock" onSeeAll={() => router.push("/(tabs)/products")}>
                {data.low_stock.map((item, i) => (
                  <RowWrapper key={item.id} divider={i > 0}>
                    <LowStockRow
                      item={item}
                      onPress={() =>
                        router.push(
                          item.product_id
                            ? `/(tabs)/products/${item.product_id}`
                            : "/(tabs)/products",
                        )
                      }
                    />
                  </RowWrapper>
                ))}
              </Section>
            ) : null}
```

Before the API deploy, `product_id` is absent and the row lands on the products list — correct behaviour, not an error. After the deploy it lands on the product.

- [ ] **Step 6: Run both gates**

```bash
cd packages/mobile-shared && npm test && npm run check-types
cd ../../apps/mobile-admin && npm test && npm run check-types
```

Expected: all pass in both packages.

- [ ] **Step 7: Commit**

```bash
git add packages/mobile-shared/api/schemas/dashboard.ts \
        packages/mobile-shared/api/schemas/__tests__/dashboard.test.ts \
        "apps/mobile-admin/app/(tabs)/index.tsx"
git commit -m "feat(mobile-shared): accept dashboard customer_name/image_url/product_id, fix low-stock nav"
```

---

## Device verification

After Task 8 and again after Task 10, run the app and check by eye. The type and density changes are global, so regressions show up as clipped text and broken alignment, which no unit test catches.

```bash
cd apps/mobile-admin && npx expo start --clear
```

Check on an iOS simulator **and** an Android emulator:

1. **Dashboard, Orders, Products, Customers, More** — the five tab roots, reachable by deep link (`mark8ly-admin://orders` etc.).
2. **Press feedback** — hold a list row: iOS shows a background shift to `#ECEAE3`, Android a ripple. Neither shows a whole-row fade.
3. **Product thumbnails** fade in rather than popping, and a product with no image shows the placeholder at the same size.
4. **Segmented control** gives a selection tick on iOS.

The 2026-07-17 pass left three screens unverified because deep links do not push nested routes — only the five tab roots. These need human taps this time, since the type re-scale affects them heavily:

- Product editor `app/(tabs)/products/[id].tsx` — tap a product row
- Product create `app/(tabs)/products/new.tsx` — tap the catalog FAB
- Customer detail `app/(tabs)/customers/[id].tsx` — tap a customer row

---

## What this plan does NOT cover

Deliberately deferred to their own plans, because they depend on primitives this plan creates:

- **Increment 2 (Slice):** Ink dock rewrite, `CollapsingHeader`, `SwipeRow`, `ActionSheet`, `RevenueChart`, `lib/queue.ts`, the Dashboard action queue, and Orders gestures.
- **Increment 3 (Rollout):** the pattern applied to Products, Customers, Reviews, Tickets, Coupons, Gift cards, Campaigns, Segments, Order detail, and More/Account/Settings.

Both are specified in `docs/superpowers/specs/2026-07-27-mobile-admin-native-ux-design.md` §2 and §3.
