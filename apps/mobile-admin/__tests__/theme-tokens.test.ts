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

  const sources = [
    path.resolve(__dirname, '../lib/theme.ts'),
    path.resolve(__dirname, '../tailwind.config.js'),
  ];

  // Strip both block and line comments from source code before searching.
  // Allows banned values to be preserved in comments (documentation) while
  // still catching them in actual code.
  function stripComments(code: string): string {
    // Remove block comments and line comments from the code.
    let result = code.replace(/\/\*[\s\S]*?\*\//g, '');
    result = result.replace(/\/\/.*$/gm, '');
    return result;
  }

  for (const file of sources) {
    it(`does not reintroduce a failing text colour in ${path.basename(file)}`, () => {
      const contents = fs.readFileSync(file, 'utf8');
      const contentsNoComments = stripComments(contents);
      for (const banned of BANNED) {
        if (contentsNoComments.includes(banned)) {
          throw new Error(
            `WCAG AA guard failed: banned colour value "${banned}" found in ${path.basename(file)} (outside comments)`,
          );
        }
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
