// `CollapsingHeader` (imported below only for its MAX_FONT_SCALE alias)
// gained a back chevron in increment 3 Task 1, so it now pulls an icon from
// lucide-react-native's ESM build, which jest-expo's default
// `transformIgnorePatterns` does not transform. Same one-line Proxy mock the
// ~20 icon-touching suites in `__tests__/` already use.
jest.mock('lucide-react-native', () => new Proxy({}, { get: () => () => null }));

import { render } from '@testing-library/react-native';
import { Text, MAX_FONT_SCALE } from './Text';
import { MAX_FONT_SCALE as HEADER_MAX_FONT_SCALE } from './CollapsingHeader';
import { MAX_FONT_SCALE as CHIP_MAX_FONT_SCALE } from './FilterChips';

// Dynamic Type, pinned at the chokepoint.
//
// Every screen's text goes through this component (it is the ONLY importer of
// react-native's own `Text` in the app), and before this default existed
// ~20 text nodes grew without bound at iOS's accessibility sizes — labels
// running under neighbouring elements, the dock's five tabs degrading to
// single letters. Three components had independently written their own
// `maxFontSizeMultiplier={2}`; everything else had nothing.
//
// Deleting the default leaves every OTHER test in this suite green, so the
// assertions have to live here.
describe('Text — Dynamic Type cap', () => {
  it('caps every text node at 200% by default', () => {
    const { getByText } = render(<Text>Revenue</Text>);
    expect(getByText('Revenue').props.maxFontSizeMultiplier).toBe(2);
    expect(MAX_FONT_SCALE).toBe(2);
  });

  it('applies the cap on every preset, not just the body default', () => {
    const presets = ['heroNumeral', 'display', 'h1', 'h2', 'h3', 'eyebrow', 'bodyLg', 'body', 'bodyEmphasis', 'label', 'caption'] as const;
    for (const preset of presets) {
      const { getByText } = render(<Text preset={preset}>{preset}</Text>);
      expect(getByText(preset).props.maxFontSizeMultiplier).toBe(MAX_FONT_SCALE);
    }
  });

  // TenantMonogram's initial pins 1 deliberately — an identity mark inside a
  // fixed 40pt disc has no content to resize. A looser default must not
  // silently overwrite a caller's tighter cap.
  it('lets a caller pin a TIGHTER cap than the default', () => {
    const { getByText } = render(<Text maxFontSizeMultiplier={1}>T</Text>);
    expect(getByText('T').props.maxFontSizeMultiplier).toBe(1);
  });

  // CollapsingHeader.headerHeightsFor and FilterChips.chipHeightsFor scale
  // their CONTAINERS by their own MAX_FONT_SCALE. If either ever drifts from
  // the multiplier the text is actually capped at, the box and its contents
  // are computed against different scales and the content clips.
  it('is the single source of truth for the two primitives that size boxes from it', () => {
    expect(HEADER_MAX_FONT_SCALE).toBe(MAX_FONT_SCALE);
    expect(CHIP_MAX_FONT_SCALE).toBe(MAX_FONT_SCALE);
  });
});

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

  it('maps the heroNumeral preset to the serif hero classes', () => {
    const { getByText } = render(<Text preset="heroNumeral">$4,280</Text>);
    const node = getByText('$4,280');
    expect(node.props.className).toContain('font-serif-bold');
    expect(node.props.className).toContain('text-hero');
    expect(node.props.className).toContain('text-ink');
  });
});
