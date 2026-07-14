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
