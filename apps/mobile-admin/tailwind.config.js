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
          // Was #7A766E (~4.18:1 on paper — fails WCAG AA). Aligned to
          // lib/theme.ts textTertiary (#5C5953, ~6.45:1) so both token
          // sources agree.
          muted: '#5C5953',
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
      borderRadius: {
        none: '0px',
        sm: '4px',
        DEFAULT: '6px',
        // Aligned to theme.ts radii.md (6px) — was 10px, colliding with the
        // same "md" name at a different value. Zero `rounded-md` usages
        // found in the app (grepped 2026-07-17), so safe to realign rather
        // than just document the drift.
        md: '6px',
        lg: '14px',
        full: '9999px',
      },
      minHeight: { touch: '44px' },
      minWidth: { touch: '44px' },
    },
  },
  plugins: [],
};
