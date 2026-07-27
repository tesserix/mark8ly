// tsconfig scopes `types` to ["jest"], so avoid importing Node's `fs` and
// `path` modules and declare the Node global this file's __dirname needs.
declare const __dirname: string;

import { theme } from '@/lib/theme';

const fs = require('fs');
const path = require('path');

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

describe('WCAG AA colour guard', () => {
  // Both of these fail 4.5:1 against Paper #F7F6F2 and were removed in the
  // 2026-07-17 design pass. See docs/superpowers/design-scan/.
  const BANNED = [
    'rgba(14, 14, 12, 0.5)',
    'rgba(14,14,12,0.5)',
    '#7A766E',
    '#7a766e',
  ];

  // Check token values by walking the object tree, ignoring comments entirely.
  // Comments become irrelevant by construction — we only check actual values.
  function findBannedValues(obj: unknown): string | null {
    if (typeof obj === 'string') {
      if (BANNED.includes(obj)) {
        return obj;
      }
    } else if (obj !== null && typeof obj === 'object') {
      for (const value of Object.values(obj)) {
        const banned = findBannedValues(value);
        if (banned) return banned;
      }
    }
    return null;
  }

  it('does not reintroduce a failing text colour in lib/theme.ts', () => {
    const banned = findBannedValues(theme.colors);
    if (banned) {
      throw new Error(
        `WCAG AA guard failed: banned colour value "${banned}" found in lib/theme.ts (theme.colors)`,
      );
    }
  });

  it('does not reintroduce a failing text colour in tailwind.config.js', () => {
    // Try to require() the config; if it fails due to nativewind/preset, fall back
    let twConfig: unknown;
    try {
      // eslint-disable-next-line @typescript-eslint/no-var-requires
      twConfig = require(path.resolve(__dirname, '../tailwind.config.js'));
    } catch {
      // If require fails, fall back to checking file text with a safer line-based approach
      const contents = fs.readFileSync(
        path.resolve(__dirname, '../tailwind.config.js'),
        'utf8',
      );

      // Only drop lines that are pure comments (nothing after comment markers)
      const lines = contents.split('\n').filter((line: string) => {
        const trimmed = line.trim();
        if (trimmed.startsWith('//')) return false;
        // For block comments, only skip if line is ONLY comment markers
        if (trimmed.startsWith('/*') && trimmed.endsWith('*/')) return false;
        if (trimmed === '*' || trimmed === '*/' || trimmed.startsWith('*')) return false;
        return true;
      });
      const safeContents = lines.join('\n');

      for (const banned of BANNED) {
        if (safeContents.includes(banned)) {
          throw new Error(
            `WCAG AA guard failed: banned colour value "${banned}" found in tailwind.config.js (outside comments)`,
          );
        }
      }
      return; // Fallback path complete
    }

    // If require() succeeded, walk the config tree
    if (typeof twConfig === 'object' && twConfig !== null && 'theme' in twConfig) {
      const themeExtend = (twConfig as Record<string, unknown>).theme;
      if (typeof themeExtend === 'object' && themeExtend !== null && 'extend' in themeExtend) {
        const colors = (themeExtend as Record<string, unknown>).extend;
        if (typeof colors === 'object' && colors !== null && 'colors' in colors) {
          const banned = findBannedValues((colors as Record<string, unknown>).colors);
          if (banned) {
            throw new Error(
              `WCAG AA guard failed: banned colour value "${banned}" found in tailwind.config.js (theme.extend.colors)`,
            );
          }
        }
      }
    }
  });

  it('keeps tertiary text at the AA-passing value', () => {
    expect(theme.colors.textTertiary).toBe('#5C5953');
  });
});
