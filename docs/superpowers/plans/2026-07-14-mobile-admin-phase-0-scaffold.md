# mobile-admin Phase 0 — Scaffold & Boot — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the EOL `apps/mobile-admin` shell with a fresh Expo 56 / RN 0.86 / React 19 app that boots on the iPhone 17 Pro simulator and renders a brand-styled (Paper · Ink · Moss, Source Serif/Sans, nativewind) login screen.

**Architecture:** Re-scaffold onto the Home-Chef vendor app shell (nativewind config, font-gated splash, provider stack) while keeping mark8ly's existing `@repo/mobile-shared` auth/api/tenant layer. Phase 0 delivers only the shell + login; domain screens land in later phases.

**Tech Stack:** Expo SDK 56, React Native 0.86, React 19.2, expo-router 6, nativewind 4.2 + Tailwind 3.4, `@expo-google-fonts/source-serif-4` + `source-sans-3`, `@react-native-firebase/*` (existing auth), TanStack Query 5, jest-expo.

## Global Constraints

- **Stack floors (no lower):** Expo `~56.0`, React Native `0.86.x`, React `19.2.x`, Reanimated `~4.x`, nativewind `4.2.x`, Tailwind `^3.4`. (Spec §Goals #2)
- **Release target:** internal / TestFlight only — no store assets, no store review work. (Spec §Non-goals)
- **Backend/auth unchanged:** GIP Firebase auth, `X-Tenant-ID` header, Bearer token, via `@repo/mobile-shared`. Do NOT adopt Home-Chef's BFF auth. (Spec §Approach — Auth seam)
- **Brand palette (exact hex):** `paper #F7F6F2`, `paper-elevated #FFFFFF`, `ink #0E0E0C`, `ink-soft #45433E`, `ink-muted #7A766E`, `moss #2D4A2B`, `signal #D94B1A`, `danger #8B2E20`, `warning #B5751F`, `border #E2DFD6`. (Spec §Approach — Swap; `packages/ui/src/styles/mark8ly-tokens.css`)
- **Fonts:** Source Serif 4 (headings, numerals, brand) + Source Sans 3 (UI/body). RN picks fonts by family name only, so register one family per weight. (Spec §Design system)
- **Light mode only** for the brand system, but keep `userInterfaceStyle` per app.config. (Spec §Design system; `.impeccable.md`)
- **Immutability / small files:** components ≤ 400 lines, no in-place mutation of shared objects. (Global coding-style rules)
- **Commits:** single-line conventional-commit messages, no signatures. (Global git rules)

## File Structure

Work happens entirely inside `mark8ly/apps/mobile-admin/`. The legacy `app/(tabs)/*`, `components/*`, and `lib/*` domain files are **left in place untouched** this phase (they are ported/restyled in Phases 1–2); Phase 0 only replaces the shell + config + login.

| File | Responsibility | Action |
|---|---|---|
| `package.json` | Expo 56 deps + shell libs | Replace |
| `babel.config.js` | babel-preset-expo + nativewind/babel | Replace |
| `metro.config.js` | monorepo resolver + withNativeWind + `@repo/mobile-shared` resolution | Replace |
| `tailwind.config.js` | Paper·Ink·Moss tokens + Source Serif/Sans families | Create |
| `global.css` | tailwind directives | Create |
| `nativewind-env.d.ts` | nativewind + `*.css` module types | Create |
| `tsconfig.json` | strict + `@repo/mobile-shared` paths + nativewind include | Replace |
| `app.config.js` | Expo config: newArch, plugins, fonts, FaceID/camera perms | Replace |
| `eas.json` | internal sim + dev build profiles | Replace |
| `lib/fonts.ts` | font family map for `useFonts` | Create |
| `components/ui/Text.tsx` | brand `Text` primitive (preset → font family + size) | Replace |
| `components/ErrorBoundary.tsx` | top-level error boundary | Create |
| `app/_layout.tsx` | provider stack + font-gated splash + auth gate | Replace |
| `app/login.tsx` | brand-styled login screen | Replace |
| `components/ui/Text.test.tsx` | Text preset unit test | Create |
| `app/login.test.tsx` | login render smoke test | Create |

---

### Task 1: Reset dependencies to the Expo 56 stack

**Files:**
- Modify: `apps/mobile-admin/package.json`

**Interfaces:**
- Produces: an installable Expo 56 workspace app; every later task depends on these versions resolving.

- [ ] **Step 1: Replace `package.json` with the Expo 56 dependency set**

```json
{
  "name": "@repo/mobile-admin",
  "version": "0.0.0",
  "private": true,
  "main": "expo-router/entry",
  "scripts": {
    "dev": "expo start",
    "ios": "expo run:ios",
    "build": "echo 'use eas build'",
    "lint": "eslint .",
    "check-types": "tsc --noEmit",
    "test": "jest"
  },
  "dependencies": {
    "@expo-google-fonts/source-sans-3": "^0.4.0",
    "@expo-google-fonts/source-serif-4": "^0.4.0",
    "@gorhom/bottom-sheet": "^5.2.8",
    "@react-native-async-storage/async-storage": "3.1.1",
    "@react-native-community/netinfo": "~12.0.1",
    "@react-native-firebase/app": "^24.0.0",
    "@react-native-firebase/auth": "^24.0.0",
    "@repo/mobile-shared": "*",
    "@tanstack/react-query": "^5.83.0",
    "expo": "~56.0.0",
    "expo-build-properties": "~56.0.0",
    "expo-camera": "~56.0.0",
    "expo-constants": "~56.0.0",
    "expo-dev-client": "~56.0.0",
    "expo-device": "~56.0.0",
    "expo-font": "~56.0.0",
    "expo-haptics": "~56.0.0",
    "expo-image": "~56.0.0",
    "expo-image-manipulator": "~56.0.0",
    "expo-image-picker": "~56.0.0",
    "expo-linking": "~56.0.0",
    "expo-local-authentication": "~56.0.0",
    "expo-notifications": "~56.0.0",
    "expo-router": "~6.0.0",
    "expo-secure-store": "~56.0.0",
    "expo-splash-screen": "~56.0.0",
    "expo-status-bar": "~56.0.0",
    "lucide-react-native": "^1.20.0",
    "nativewind": "4.2.5",
    "react": "19.2.0",
    "react-dom": "19.2.0",
    "react-native": "0.86.0",
    "react-native-gesture-handler": "~3.0.1",
    "react-native-reanimated": "~4.4.1",
    "react-native-safe-area-context": "5.8.0",
    "react-native-screens": "4.25.2",
    "react-native-svg": "~15.13.0",
    "react-native-worklets": "0.9.2",
    "tailwindcss": "^3.4.17",
    "zod": "^4.4.3",
    "zustand": "^5.0.0"
  },
  "devDependencies": {
    "@testing-library/react-native": "^13.0.0",
    "@types/react": "~19.2.0",
    "jest": "^30.3.0",
    "jest-expo": "~56.0.0",
    "typescript": "^5.9.0"
  }
}
```

> Note: exact patch versions are pinned by `expo install --fix` in Step 3; the `~56.0.0` entries let Expo resolve the SDK-correct patch. If any `@expo-google-fonts/*` `^0.4.0` is unavailable, use the latest published `0.x` — the family export names in Task 4 are stable across `0.x`.

- [ ] **Step 2: Install from the app directory**

Run: `cd apps/mobile-admin && npm install --workspaces=false --legacy-peer-deps`
Expected: completes without an `ERESOLVE` hard failure. If the root `ajv`/`ajv-keywords` conflict resurfaces, run `npm dedupe ajv` at the mark8ly root, then reinstall.

- [ ] **Step 3: Let Expo pin SDK-correct versions**

Run: `cd apps/mobile-admin && npx expo install --fix`
Expected: rewrites any mismatched RN/Expo patch versions; exits 0.

- [ ] **Step 4: Verify the Expo config resolves**

Run: `cd apps/mobile-admin && npx expo config --type public > /dev/null && echo OK`
Expected: prints `OK` (config loads without the `ajv` prebuild error).

- [ ] **Step 5: Commit**

```bash
git add apps/mobile-admin/package.json apps/mobile-admin/package-lock.json
git commit -m "chore(mobile-admin): reset deps to Expo 56 / RN 0.86 / React 19 stack"
```

---

### Task 2: Wire nativewind + monorepo metro + tsconfig

**Files:**
- Create: `apps/mobile-admin/global.css`
- Create: `apps/mobile-admin/nativewind-env.d.ts`
- Modify: `apps/mobile-admin/babel.config.js`
- Modify: `apps/mobile-admin/metro.config.js`
- Modify: `apps/mobile-admin/tsconfig.json`

**Interfaces:**
- Produces: `import '../global.css'` side-effect resolves; `@repo/mobile-shared` and `@repo/mobile-shared/*` resolve via metro + tsconfig paths; nativewind `className` compiles.

- [ ] **Step 1: Create `global.css`**

```css
@tailwind base;
@tailwind components;
@tailwind utilities;
```

- [ ] **Step 2: Create `nativewind-env.d.ts`**

```typescript
/// <reference types="nativewind/types" />

// NativeWind 4.2 / RN 0.86 no longer declares a side-effect module for
// `*.css`, so `import '../global.css'` reports TS2882. Declare it here.
declare module '*.css' {}
```

- [ ] **Step 3: Replace `babel.config.js`**

```javascript
module.exports = function (api) {
  api.cache(true);
  return {
    presets: ['babel-preset-expo', 'nativewind/babel'],
  };
};
```

- [ ] **Step 4: Replace `metro.config.js`**

```javascript
const { getDefaultConfig } = require('expo/metro-config');
const { withNativeWind } = require('nativewind/metro');
const { FileStore } = require('metro-cache');
const path = require('path');

const projectRoot = __dirname;
const monorepoRoot = path.resolve(projectRoot, '../..');

const config = getDefaultConfig(projectRoot);

config.watchFolders = [monorepoRoot];
config.resolver.nodeModulesPaths = [
  path.resolve(projectRoot, 'node_modules'),
  path.resolve(monorepoRoot, 'node_modules'),
];
config.resolver.unstable_enableSymlinks = true;
// @repo/mobile-shared ships subpath exports (api/client, auth/provider, …),
// so package-exports resolution must stay on.
config.resolver.unstable_enablePackageExports = true;

config.cacheStores = [
  new FileStore({ root: path.join(projectRoot, '.metro-cache') }),
];

module.exports = withNativeWind(config, {
  input: './global.css',
});
```

- [ ] **Step 5: Replace `tsconfig.json`**

```json
{
  "extends": "expo/tsconfig.base",
  "compilerOptions": {
    "strict": true,
    "paths": {
      "@repo/mobile-shared": ["../../packages/mobile-shared"],
      "@repo/mobile-shared/*": ["../../packages/mobile-shared/*"]
    }
  },
  "include": [
    "**/*.ts",
    "**/*.tsx",
    ".expo/types/**/*.d.ts",
    "expo-env.d.ts",
    "nativewind-env.d.ts"
  ]
}
```

- [ ] **Step 6: Verify metro starts and bundles the entry**

Run: `cd apps/mobile-admin && npx expo export --platform ios --output-dir /tmp/mp-export-check > /dev/null 2>&1 && echo BUNDLED`
Expected: prints `BUNDLED` (metro resolves nativewind + `@repo/mobile-shared` without a "module not found").

- [ ] **Step 7: Commit**

```bash
git add apps/mobile-admin/global.css apps/mobile-admin/nativewind-env.d.ts \
  apps/mobile-admin/babel.config.js apps/mobile-admin/metro.config.js apps/mobile-admin/tsconfig.json
git commit -m "chore(mobile-admin): wire nativewind + monorepo metro + tsconfig paths"
```

---

### Task 3: Tailwind theme — Paper · Ink · Moss + Source Serif/Sans

**Files:**
- Create: `apps/mobile-admin/tailwind.config.js`

**Interfaces:**
- Produces: nativewind utility classes `bg-paper`, `text-ink`, `text-ink-soft`, `bg-moss`, `text-signal`, `border-border`, `font-serif`, `font-serif-bold`, `font-sans`, `font-sans-medium`, `font-sans-semibold`, and `text-display/h1/h2/body/body-sm/label/caption`. Task 4's `Text` primitive consumes these class names.

- [ ] **Step 1: Create `tailwind.config.js`** (values copied verbatim from `packages/ui/src/styles/mark8ly-tokens.css`)

```javascript
/** @type {import('tailwindcss').Config} */
//
// Source of truth: packages/ui/src/styles/mark8ly-tokens.css (Paper · Ink · Moss).
// RN picks fonts by family name only, so each weight is its own family —
// loaded via expo-font in lib/fonts.ts + app/_layout.tsx.
module.exports = {
  darkMode: 'media',
  content: [
    './app/**/*.{js,ts,jsx,tsx}',
    './components/**/*.{js,ts,jsx,tsx}',
    './lib/**/*.{js,ts,jsx,tsx}',
    '../../packages/mobile-shared/**/*.{js,ts,jsx,tsx}',
  ],
  presets: [require('nativewind/preset')],
  theme: {
    extend: {
      colors: {
        paper: {
          DEFAULT: '#F7F6F2',
          elevated: '#FFFFFF',
          sink: '#ECEAE3',
        },
        ink: {
          DEFAULT: '#0E0E0C',
          soft: '#45433E',
          muted: '#7A766E',
          faint: '#A09C92',
        },
        moss: {
          DEFAULT: '#2D4A2B',
          soft: '#3D5F38',
          tint: '#E8EEE2',
        },
        // Functional only — never decorative.
        signal: '#D94B1A',
        danger: {
          DEFAULT: '#8B2E20',
          tint: '#F6E4E1',
        },
        warning: {
          DEFAULT: '#B5751F',
          tint: '#F3E7CE',
        },
        border: {
          DEFAULT: '#E2DFD6',
          strong: '#C7C3B8',
          subtle: '#ECEAE3',
        },
        background: '#F7F6F2',
        foreground: '#0E0E0C',
      },
      fontFamily: {
        // font-sans          → body / UI              (Source Sans 3 400)
        // font-sans-medium   → slight emphasis        (Source Sans 3 500)
        // font-sans-semibold → buttons, strong labels (Source Sans 3 600)
        // font-serif         → headlines, numerals    (Source Serif 4 600)
        // font-serif-bold    → hero display           (Source Serif 4 700)
        sans: ['SourceSans', 'System'],
        'sans-medium': ['SourceSans-Medium', 'SourceSans', 'System'],
        'sans-semibold': ['SourceSans-SemiBold', 'SourceSans', 'System'],
        serif: ['SourceSerif', 'Georgia', 'serif'],
        'serif-bold': ['SourceSerif-Bold', 'SourceSerif', 'serif'],
        mono: ['Menlo', 'Courier New'],
      },
      fontSize: {
        display: ['36px', { lineHeight: '42px', letterSpacing: '-0.5px' }],
        h1: ['28px', { lineHeight: '34px', letterSpacing: '-0.3px' }],
        h2: ['22px', { lineHeight: '28px', letterSpacing: '-0.2px' }],
        h3: ['18px', { lineHeight: '24px', letterSpacing: '0px' }],
        'body-lg': ['16px', { lineHeight: '22px', letterSpacing: '0px' }],
        body: ['14px', { lineHeight: '20px', letterSpacing: '0px' }],
        label: ['13px', { lineHeight: '18px', letterSpacing: '0.1px' }],
        caption: ['12px', { lineHeight: '16px', letterSpacing: '0.2px' }],
        eyebrow: ['11px', { lineHeight: '14px', letterSpacing: '1.2px' }],
      },
      borderRadius: {
        none: '0px',
        sm: '4px',
        DEFAULT: '6px',
        md: '10px',
        lg: '14px',
        full: '9999px',
      },
      minHeight: { touch: '44px' },
      minWidth: { touch: '44px' },
    },
  },
  plugins: [],
};
```

- [ ] **Step 2: Verify the config parses**

Run: `cd apps/mobile-admin && node -e "require('./tailwind.config.js'); console.log('TAILWIND OK')"`
Expected: prints `TAILWIND OK`.

- [ ] **Step 3: Commit**

```bash
git add apps/mobile-admin/tailwind.config.js
git commit -m "feat(mobile-admin): Paper·Ink·Moss + Source Serif/Sans nativewind theme"
```

---

### Task 4: Brand `Text` primitive (TDD) + font map

**Files:**
- Create: `apps/mobile-admin/lib/fonts.ts`
- Modify: `apps/mobile-admin/components/ui/Text.tsx`
- Create: `apps/mobile-admin/components/ui/Text.test.tsx`

**Interfaces:**
- Consumes: nativewind classes from Task 3.
- Produces:
  - `lib/fonts.ts` → `export const fontMap: Record<string, number>` (passed to `useFonts`), covering keys `SourceSerif`, `SourceSerif-Bold`, `SourceSans`, `SourceSans-Medium`, `SourceSans-SemiBold`.
  - `components/ui/Text.tsx` → `export type TextPreset = 'display' | 'h1' | 'h2' | 'h3' | 'eyebrow' | 'bodyLg' | 'body' | 'bodyEmphasis' | 'caption'` and `export function Text(props: { preset?: TextPreset; className?: string; children: React.ReactNode } & RNTextProps)`. Every later screen renders copy through this component.

- [ ] **Step 1: Create the font map `lib/fonts.ts`**

```typescript
import { SourceSerif4_600SemiBold } from '@expo-google-fonts/source-serif-4/600SemiBold';
import { SourceSerif4_700Bold } from '@expo-google-fonts/source-serif-4/700Bold';
import { SourceSans3_400Regular } from '@expo-google-fonts/source-sans-3/400Regular';
import { SourceSans3_500Medium } from '@expo-google-fonts/source-sans-3/500Medium';
import { SourceSans3_600SemiBold } from '@expo-google-fonts/source-sans-3/600SemiBold';

// RN selects a font by family name only (fontWeight does not pick a
// different family for non-system fonts), so each weight is registered as
// its own family. Keys MUST match the fontFamily names in tailwind.config.js.
export const fontMap = {
  SourceSerif: SourceSerif4_600SemiBold,
  'SourceSerif-Bold': SourceSerif4_700Bold,
  SourceSans: SourceSans3_400Regular,
  'SourceSans-Medium': SourceSans3_500Medium,
  'SourceSans-SemiBold': SourceSans3_600SemiBold,
};
```

- [ ] **Step 2: Write the failing test `components/ui/Text.test.tsx`**

```tsx
import { render } from '@testing-library/react-native';
import { Text } from './Text';

describe('Text', () => {
  it('maps the h1 preset to serif display classes', () => {
    const { getByText } = render(<Text preset="h1">Orders</Text>);
    const node = getByText('Orders');
    expect(node.props.className).toContain('font-serif');
    expect(node.props.className).toContain('text-h1');
    expect(node.props.className).toContain('text-ink');
  });

  it('defaults to the sans body preset', () => {
    const { getByText } = render(<Text>Body copy</Text>);
    const node = getByText('Body copy');
    expect(node.props.className).toContain('font-sans');
    expect(node.props.className).toContain('text-body');
  });

  it('appends a caller className after the preset classes', () => {
    const { getByText } = render(
      <Text preset="caption" className="text-ink-muted">
        Meta
      </Text>,
    );
    const node = getByText('Meta');
    expect(node.props.className).toContain('text-caption');
    expect(node.props.className).toContain('text-ink-muted');
  });
});
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd apps/mobile-admin && npx jest components/ui/Text.test.tsx`
Expected: FAIL — current `Text.tsx` is the legacy `theme.ts` version and has no `className`-based preset mapping.

- [ ] **Step 4: Replace `components/ui/Text.tsx` with the nativewind version**

```tsx
import { Text as RNText, type TextProps as RNTextProps } from 'react-native';

export type TextPreset =
  | 'display'
  | 'h1'
  | 'h2'
  | 'h3'
  | 'eyebrow'
  | 'bodyLg'
  | 'body'
  | 'bodyEmphasis'
  | 'caption';

// Each preset is a fixed set of nativewind classes: family + size + default
// color. Callers pass `className` to override color/spacing per use.
const PRESET_CLASSES: Record<TextPreset, string> = {
  display: 'font-serif-bold text-display text-ink',
  h1: 'font-serif text-h1 text-ink',
  h2: 'font-serif text-h2 text-ink',
  h3: 'font-serif text-h3 text-ink',
  eyebrow: 'font-sans-semibold text-eyebrow uppercase text-ink-muted',
  bodyLg: 'font-sans text-body-lg text-ink',
  body: 'font-sans text-body text-ink',
  bodyEmphasis: 'font-sans-semibold text-body text-ink',
  caption: 'font-sans-medium text-caption text-ink-soft',
};

export interface TextComponentProps extends RNTextProps {
  preset?: TextPreset;
  className?: string;
}

export function Text({
  preset = 'body',
  className,
  ...rest
}: TextComponentProps) {
  const merged = className
    ? `${PRESET_CLASSES[preset]} ${className}`
    : PRESET_CLASSES[preset];
  return <RNText className={merged} {...rest} />;
}
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd apps/mobile-admin && npx jest components/ui/Text.test.tsx`
Expected: PASS (3 tests).

- [ ] **Step 6: Commit**

```bash
git add apps/mobile-admin/lib/fonts.ts apps/mobile-admin/components/ui/Text.tsx apps/mobile-admin/components/ui/Text.test.tsx
git commit -m "feat(mobile-admin): nativewind Text primitive + Source font map"
```

---

### Task 5: Error boundary + root layout with provider stack

**Files:**
- Create: `apps/mobile-admin/components/ErrorBoundary.tsx`
- Modify: `apps/mobile-admin/app/_layout.tsx`

**Interfaces:**
- Consumes: `fontMap` (Task 4); `@repo/mobile-shared` `AuthProvider` + `useAuth`; `useTenantStore` from `@repo/mobile-shared/stores/tenant-store`.
- Produces: a mounted provider tree that gates rendering on fonts + auth and routes unauthenticated users to `/login`.

- [ ] **Step 1: Create `components/ErrorBoundary.tsx`**

```tsx
import { Component, type ReactNode } from 'react';
import { View } from 'react-native';
import { Text } from './ui/Text';

interface Props {
  children: ReactNode;
}
interface State {
  error: Error | null;
}

// Top-level boundary so a render error surfaces a readable screen instead of
// a white crash. Logged to console for now; wired to telemetry in Phase 4.
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error) {
    console.error('[mobile-admin] render error', error);
  }

  render() {
    if (this.state.error) {
      return (
        <View className="flex-1 items-center justify-center bg-paper px-6">
          <Text preset="h2" className="text-center">
            Something went wrong
          </Text>
          <Text preset="body" className="mt-2 text-center text-ink-muted">
            Restart the app to continue.
          </Text>
        </View>
      );
    }
    return this.props.children;
  }
}
```

- [ ] **Step 2: Replace `app/_layout.tsx`**

```tsx
import '../global.css';

import { useEffect, useRef } from 'react';
import { View } from 'react-native';
import { Slot, useRouter, useSegments } from 'expo-router';
import { GestureHandlerRootView } from 'react-native-gesture-handler';
import { BottomSheetModalProvider } from '@gorhom/bottom-sheet';
import {
  QueryClient,
  QueryClientProvider,
  useQueryClient,
} from '@tanstack/react-query';
import * as SplashScreen from 'expo-splash-screen';
import { useFonts } from 'expo-font';
import { AuthProvider, useAuth } from '@repo/mobile-shared/auth/provider';
import { useTenantStore } from '@repo/mobile-shared/stores/tenant-store';
import { ErrorBoundary } from '../components/ErrorBoundary';
import { fontMap } from '../lib/fonts';

SplashScreen.preventAutoHideAsync();

const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 30_000, retry: 2 } },
});

function AuthGate() {
  const { user, loading } = useAuth();
  const segments = useSegments();
  const router = useRouter();
  const qc = useQueryClient();
  const hydrate = useTenantStore((s) => s.hydrate);
  const clearTenant = useTenantStore((s) => s.clear);
  const previousUid = useRef<string | null>(null);

  useEffect(() => {
    hydrate();
  }, [hydrate]);

  useEffect(() => {
    if (loading) return;
    const inAuthGroup = segments[0] === 'login';

    // Identity changed → wipe cached queries + tenant so user A's data can't
    // leak into user B's session.
    const currentUid = user?.uid ?? null;
    if (previousUid.current !== null && previousUid.current !== currentUid) {
      qc.clear();
      clearTenant();
    }
    previousUid.current = currentUid;

    if (!user && !inAuthGroup) {
      router.replace('/login');
    } else if (user && inAuthGroup) {
      router.replace('/');
    }
    SplashScreen.hideAsync();
  }, [user, loading, segments, router, clearTenant, qc]);

  return <Slot />;
}

export default function RootLayout() {
  const [fontsLoaded] = useFonts(fontMap);
  if (!fontsLoaded) return <View className="flex-1 bg-paper" />;

  return (
    <ErrorBoundary>
      <AuthProvider>
        <GestureHandlerRootView style={{ flex: 1 }}>
          <QueryClientProvider client={queryClient}>
            <BottomSheetModalProvider>
              <AuthGate />
            </BottomSheetModalProvider>
          </QueryClientProvider>
        </GestureHandlerRootView>
      </AuthProvider>
    </ErrorBoundary>
  );
}
```

> If `@repo/mobile-shared`'s `AuthProvider` requires props (e.g. `tenantId`), read `packages/mobile-shared/auth/provider` first and pass the values from `expo-constants` `extra` (set in Task 6's `app.config.js`). Do NOT hardcode secrets.

- [ ] **Step 3: Type-check**

Run: `cd apps/mobile-admin && npx tsc --noEmit`
Expected: no errors from `_layout.tsx` or `ErrorBoundary.tsx`. (Legacy `(tabs)` screens may still type-check; if a legacy file errors on the new React 19 types, that's expected and handled in Phase 2 — confirm the error is NOT in the Phase 0 files.)

- [ ] **Step 4: Commit**

```bash
git add apps/mobile-admin/components/ErrorBoundary.tsx apps/mobile-admin/app/_layout.tsx
git commit -m "feat(mobile-admin): provider stack + font-gated auth root layout"
```

---

### Task 6: Brand-styled login screen (+ render test)

**Files:**
- Modify: `apps/mobile-admin/app/login.tsx`
- Create: `apps/mobile-admin/app/login.test.tsx`

**Interfaces:**
- Consumes: `Text` primitive (Task 4); `useAuth` from `@repo/mobile-shared/auth/provider`.
- Produces: the `/login` route — the only interactive screen this phase.

- [ ] **Step 1: Write the failing render test `app/login.test.tsx`**

```tsx
import { render } from '@testing-library/react-native';
import LoginScreen from './login';

jest.mock('@repo/mobile-shared/auth/provider', () => ({
  useAuth: () => ({ signIn: jest.fn(), loading: false }),
}));

describe('LoginScreen', () => {
  it('renders the brand wordmark and a sign-in action', () => {
    const { getByText, getByLabelText } = render(<LoginScreen />);
    expect(getByText('Mark8ly')).toBeTruthy();
    expect(getByLabelText('Sign in')).toBeTruthy();
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd apps/mobile-admin && npx jest app/login.test.tsx`
Expected: FAIL — legacy `login.tsx` uses the old theme and lacks the `Sign in` accessibility label / wordmark.

- [ ] **Step 3: Replace `app/login.tsx`**

```tsx
import { useState } from 'react';
import { Pressable, TextInput, View } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useAuth } from '@repo/mobile-shared/auth/provider';
import { Text } from '../components/ui/Text';

export default function LoginScreen() {
  const { signIn, loading } = useAuth();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');

  return (
    <SafeAreaView className="flex-1 bg-paper">
      <View className="flex-1 justify-center px-6">
        <Text preset="eyebrow" className="text-moss">
          Merchant admin
        </Text>
        <Text preset="display" className="mt-2">
          Mark8ly
        </Text>
        <Text preset="body" className="mt-2 text-ink-muted">
          Sign in to manage your store.
        </Text>

        <View className="mt-8 gap-3">
          <TextInput
            accessibilityLabel="Email"
            className="min-h-touch rounded border border-border bg-paper-elevated px-4 font-sans text-body text-ink"
            placeholder="Email"
            placeholderTextColor="#7A766E"
            autoCapitalize="none"
            keyboardType="email-address"
            value={email}
            onChangeText={setEmail}
          />
          <TextInput
            accessibilityLabel="Password"
            className="min-h-touch rounded border border-border bg-paper-elevated px-4 font-sans text-body text-ink"
            placeholder="Password"
            placeholderTextColor="#7A766E"
            secureTextEntry
            value={password}
            onChangeText={setPassword}
          />
        </View>

        <Pressable
          accessibilityRole="button"
          accessibilityLabel="Sign in"
          disabled={loading}
          onPress={() => signIn(email, password)}
          className="mt-6 min-h-touch items-center justify-center rounded bg-ink active:opacity-90"
        >
          <Text preset="bodyEmphasis" className="text-paper">
            {loading ? 'Signing in…' : 'Sign in'}
          </Text>
        </Pressable>
      </View>
    </SafeAreaView>
  );
}
```

> Read `packages/mobile-shared/auth/provider` for the exact `signIn` signature before wiring — if it is `signIn({ email, password })` rather than positional, adapt the `onPress` call. The test mocks `useAuth`, so it stays green either way; fix the real call to match.

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd apps/mobile-admin && npx jest app/login.test.tsx`
Expected: PASS (1 test).

- [ ] **Step 5: Commit**

```bash
git add apps/mobile-admin/app/login.tsx apps/mobile-admin/app/login.test.tsx
git commit -m "feat(mobile-admin): brand-styled login screen"
```

---

### Task 7: Expo app config + EAS + native permissions

**Files:**
- Modify: `apps/mobile-admin/app.config.js`
- Modify: `apps/mobile-admin/eas.json`

**Interfaces:**
- Produces: a prebuild-able native config (new architecture, required plugins, FaceID/camera permission strings) and internal sim/dev EAS profiles. Task 8 consumes this to build.

- [ ] **Step 1: Replace `app.config.js`**

```javascript
const PRODUCTION = {
  name: 'Mark8ly Admin',
  bundleIdentifier: 'com.mark8ly.admin',
  androidPackage: 'com.mark8ly.admin',
  extra: {
    apiBaseUrl: process.env.EXPO_PUBLIC_API_URL || 'https://api.mark8ly.com',
    storefrontUrl: 'https://mark8ly.com',
    adminWebUrl: 'https://admin.mark8ly.com',
    signupUrl: 'https://mark8ly.com',
    gipTenantId: process.env.GIP_TENANT_ID || '',
  },
};

module.exports = {
  expo: {
    name: PRODUCTION.name,
    slug: 'mark8ly-admin',
    scheme: 'mark8ly-admin',
    version: '1.0.0',
    orientation: 'portrait',
    icon: './assets/icon.png',
    userInterfaceStyle: 'light',
    newArchEnabled: true,
    jsEngine: 'hermes',
    splash: {
      image: './assets/splash.png',
      resizeMode: 'contain',
      backgroundColor: '#F7F6F2',
    },
    ios: {
      supportsTablet: false,
      bundleIdentifier: PRODUCTION.bundleIdentifier,
      infoPlist: {
        NSFaceIDUsageDescription: 'Use Face ID to unlock the admin app',
        NSCameraUsageDescription: 'Take product photos for your store',
        NSPhotoLibraryUsageDescription:
          'Select product images from your library',
        ITSAppUsesNonExemptEncryption: false,
      },
      associatedDomains: ['applinks:admin.mark8ly.com'],
    },
    android: {
      adaptiveIcon: {
        foregroundImage: './assets/adaptive-icon.png',
        backgroundColor: '#F7F6F2',
      },
      package: PRODUCTION.androidPackage,
      intentFilters: [
        {
          action: 'VIEW',
          autoVerify: true,
          data: [
            { scheme: 'https', host: 'admin.mark8ly.com', pathPrefix: '/' },
          ],
          category: ['BROWSABLE', 'DEFAULT'],
        },
      ],
    },
    plugins: [
      'expo-router',
      'expo-font',
      'expo-secure-store',
      'expo-local-authentication',
      ['expo-camera', { cameraPermission: 'Take product photos for your store' }],
      'expo-image-picker',
      'expo-notifications',
      ['expo-build-properties', { ios: { newArchEnabled: true } }],
      '@react-native-firebase/app',
    ],
    extra: {
      eas: { projectId: 'your-eas-project-id' },
      ...PRODUCTION.extra,
    },
  },
};
```

- [ ] **Step 2: Replace `eas.json`**

```json
{
  "cli": {
    "version": ">= 18.0.0",
    "appVersionSource": "remote"
  },
  "build": {
    "local-sim": {
      "developmentClient": false,
      "distribution": "internal",
      "ios": {
        "resourceClass": "m-medium",
        "simulator": true
      },
      "env": {
        "EXPO_PUBLIC_API_URL": "https://api.mark8ly.com"
      }
    },
    "development": {
      "developmentClient": true,
      "distribution": "internal",
      "ios": { "resourceClass": "m-medium" },
      "android": { "buildType": "apk" },
      "env": {
        "EXPO_PUBLIC_API_URL": "https://api.mark8ly.com"
      }
    },
    "preview": {
      "distribution": "internal",
      "channel": "preview",
      "ios": { "resourceClass": "m-medium" },
      "env": { "EXPO_PUBLIC_API_URL": "https://api.mark8ly.com" }
    }
  }
}
```

- [ ] **Step 3: Verify config still resolves after edits**

Run: `cd apps/mobile-admin && npx expo config --type public > /dev/null && node -e "require('./eas.json'); console.log('EAS OK')"`
Expected: prints `EAS OK`.

- [ ] **Step 4: Commit**

```bash
git add apps/mobile-admin/app.config.js apps/mobile-admin/eas.json
git commit -m "chore(mobile-admin): new-arch app config, plugins, FaceID/camera perms, sim EAS profiles"
```

---

### Task 8: Boot on the iPhone 17 Pro simulator

**Files:** none (build + manual verification)

**Interfaces:**
- Consumes: everything above.
- Produces: a running app on the iPhone 17 Pro sim showing the brand login screen — the Phase 0 exit criterion and the answer to the original "run it on the sim" request.

- [ ] **Step 1: Confirm the simulator is booted**

Run: `xcrun simctl list devices | grep "iPhone 17 Pro (" | head -1`
Expected: shows `(Booted)`. If `Shutdown`, run `xcrun simctl boot "iPhone 17 Pro" && open -a Simulator`.

- [ ] **Step 2: Prebuild + build + install on the sim**

Run: `cd apps/mobile-admin && npx expo run:ios --device "iPhone 17 Pro"`
Expected: CocoaPods installs, Xcode build succeeds (Expo 56 supports Xcode 26 / iOS 26), app launches. This step is slow (5–15 min) and generates `apps/mobile-admin/ios/` (git-ignored by Expo prebuild).
Failure playbook:
- Pod install error → `cd ios && pod install --repo-update`.
- Duplicate React/RN → confirm `metro.config.js` has NO legacy `FORCE` React-pin block (removed in Task 2); a single React 19 at root is correct now.
- Firebase native error → confirm `@react-native-firebase/app` is in `app.config.js` `plugins`.

- [ ] **Step 3: Manual smoke — verify the login screen renders correctly**

Observe on the simulator:
- Background is warm Paper `#F7F6F2` (not white, not cream).
- "Mark8ly" wordmark renders in **Source Serif** (serif, not the system sans).
- "Merchant admin" eyebrow is uppercase moss `#2D4A2B`.
- The "Sign in" button is solid Ink `#0E0E0C` with Paper text and a ≥44pt tap height.
- No red error overlay; no font-flash to System sans after load.

- [ ] **Step 4: Capture proof + record the result**

Run: `xcrun simctl io "iPhone 17 Pro" screenshot /tmp/mobile-admin-login-boot.png && echo SHOT`
Expected: prints `SHOT`; open the PNG to confirm the checklist in Step 3. Attach/link it in the phase hand-off.

- [ ] **Step 5: Run the full Phase 0 test suite**

Run: `cd apps/mobile-admin && npx jest`
Expected: PASS — `Text.test.tsx` (3) + `login.test.tsx` (1).

- [ ] **Step 6: Commit the verification note**

```bash
git commit --allow-empty -m "test(mobile-admin): Phase 0 boots on iPhone 17 Pro sim (login renders in brand system)"
```

---

## Self-Review

**Spec coverage (Phase 0 scope only):**
- Modernise stack (Expo 56/RN 0.86/React 19) → Tasks 1, 7, 8. ✓
- nativewind + brand tokens + Source Serif/Sans → Tasks 2, 3, 4. ✓
- Provider stack lifted from Home-Chef, mark8ly auth kept → Task 5. ✓
- Boot on iPhone 17 Pro (Phase 0 exit criterion) → Task 8. ✓
- Testing seam established (jest-expo) → Tasks 4, 6, 8. ✓
- Later-phase concerns (domain port, Orders/Products/Customers, Marketing/Settings/Stores, rich push/biometric/i18n/force-upgrade, Maestro E2E) → intentionally deferred to Phases 1–5; not in this plan.

**Placeholder scan:** No `TBD`/`TODO`/"add error handling"; the two "read the real signature before wiring" notes point at concrete files (`packages/mobile-shared/auth/provider`) with a fallback, not vague hand-waving.

**Type consistency:** `fontMap` keys (`SourceSerif`, `SourceSerif-Bold`, `SourceSans`, `SourceSans-Medium`, `SourceSans-SemiBold`) match the `fontFamily` names in `tailwind.config.js` and the `PRESET_CLASSES` families in `Text.tsx`. `TextPreset` union in Task 4 is the one consumed by `login.tsx` in Task 6. `@repo/mobile-shared` import paths match between `metro.config.js`, `tsconfig.json`, `_layout.tsx`, and `login.tsx`.
