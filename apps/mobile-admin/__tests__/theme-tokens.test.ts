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
