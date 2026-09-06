import type { ReactNode } from "react";
import { KeyboardAvoidingView, Platform, ScrollView, View } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";

/**
 * The shell every signed-out screen sits in: /login and /otp.
 *
 * Extracted because those two had each hand-rolled the same four nested
 * containers, and the pair drifted twice in a single day — #751 top-aligned
 * the OTP screen, then #752 had to restore its top margin after NativeWind
 * silently dropped `className="flex-1"` on KeyboardAvoidingView. Two copies of
 * a layout that must be identical is the actual defect; one component is the
 * fix, and a future keyboard-avoidance change now lands on both screens or
 * neither.
 *
 * This is a pure extraction: the tree below is byte-for-byte what login.tsx
 * rendered, including the deliberate inline `style={{ flex: 1 }}`.
 */
export function AuthScreen({ children }: { children: ReactNode }) {
  return (
    <SafeAreaView className="flex-1 bg-paper">
      {/* style={{flex:1}}, NOT className="flex-1". NativeWind's interop does
          not reliably apply a className to KeyboardAvoidingView, so the
          container never fills its parent and the content collapses upward
          with no top margin. */}
      <KeyboardAvoidingView
        style={{ flex: 1 }}
        behavior={Platform.OS === "ios" ? "padding" : "height"}
      >
        <ScrollView
          contentContainerStyle={{ flexGrow: 1 }}
          keyboardShouldPersistTaps="handled"
        >
          <View className="flex-1 px-6 pt-16">{children}</View>
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}
