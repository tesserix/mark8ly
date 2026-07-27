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

  it('maps the heroNumeral preset to the serif hero classes', () => {
    const { getByText } = render(<Text preset="heroNumeral">$4,280</Text>);
    const node = getByText('$4,280');
    expect(node.props.className).toContain('font-serif-bold');
    expect(node.props.className).toContain('text-hero');
    expect(node.props.className).toContain('text-ink');
  });
});
