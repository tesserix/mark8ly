import { SourceSerif4_600SemiBold } from '@expo-google-fonts/source-serif-4/600SemiBold';
import { SourceSerif4_700Bold } from '@expo-google-fonts/source-serif-4/700Bold';
import { SourceSans3_400Regular } from '@expo-google-fonts/source-sans-3/400Regular';
import { SourceSans3_500Medium } from '@expo-google-fonts/source-sans-3/500Medium';
import { SourceSans3_600SemiBold } from '@expo-google-fonts/source-sans-3/600SemiBold';

// RN selects a font by family name only (fontWeight does not pick a
// different family for non-system fonts), so each weight is registered as
// its own family. Keys MUST match the fontFamily names in tailwind.config.js.
export const fontMap: Record<string, number> = {
  SourceSerif: SourceSerif4_600SemiBold,
  'SourceSerif-Bold': SourceSerif4_700Bold,
  SourceSans: SourceSans3_400Regular,
  'SourceSans-Medium': SourceSans3_500Medium,
  'SourceSans-SemiBold': SourceSans3_600SemiBold,
};

/**
 * The registered family NativeWind's `font-sans` Tailwind class resolves to
 * (tailwind.config.js `fontFamily.sans[0]`) — i.e. exactly what
 * `<Text preset="body">` (components/ui/Text.tsx, PRESET_CLASSES.body =
 * "font-sans ...") renders through.
 *
 * `TextInput` cannot take a NativeWind `className` the way `<Text>` does
 * (RN only applies style objects to it), so `FieldInput` and `SearchField`
 * import this constant directly instead of reaching for `theme.fonts.sans`
 * — which is the OS system font (`Platform.select({ios:"System",
 * android:"Roboto"})`, see lib/theme.ts) and is NOT what `<Text>` renders.
 *
 * `lib/fonts.test.ts` asserts this literal stays equal to
 * tailwind.config.js's actual `fontFamily.sans[0]` so the two sources of
 * truth can't silently drift apart.
 */
export const BODY_FONT_FAMILY: keyof typeof fontMap = 'SourceSans';
