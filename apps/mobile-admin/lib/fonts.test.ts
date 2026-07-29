import { fontMap, BODY_FONT_FAMILY } from './fonts';

// eslint-disable-next-line @typescript-eslint/no-var-requires
const tailwindConfig = require('../tailwind.config.js');

describe('BODY_FONT_FAMILY', () => {
  it('stays in sync with tailwind.config.js fontFamily.sans[0] — the family NativeWind\'s font-sans class (and so <Text preset="body">) actually resolves to', () => {
    expect(BODY_FONT_FAMILY).toBe(tailwindConfig.theme.extend.fontFamily.sans[0]);
  });

  it('is a family that is actually registered with expo-font', () => {
    expect(Object.keys(fontMap)).toContain(BODY_FONT_FAMILY);
  });
});
