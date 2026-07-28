// Dock — floating bottom navigation for mark8ly admin.
//
// A detached rounded bar hovering above the home indicator, filled solid Ink.
// The app's surfaces are all one value (paper on paper), so the dock is the
// single place the design is deliberately loud: a dark bar gives the screen a
// base to sit on and keeps moss free for on-screen actions.
//
// Every tab is labelled — icon-only navigation is a guess. The active tab is
// marked by a FILLED icon at full-opacity paper; inactive tabs are outline
// icons at 60% paper. There is no width-changing pill, so nothing shifts
// sideways when you switch tab.
//
// No moss dot on the active tab: the filled icon + full-opacity label already
// carry the state, and a permanent moss mark in the chrome would put a second
// moss element on screen whenever a view also shows a moss primary action
// (one-accent-per-view).
//
// Rendered via expo-router's <Tabs tabBar={...}>. Root View is absolutely
// positioned so scenes get full height and content scrolls through the gap
// beneath — screens pad their scroll bottom with useDockClearance().

import { Pressable, StyleSheet, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import Animated, {
  Easing,
  FadeIn,
  useReducedMotion,
} from "react-native-reanimated";
import {
  LayoutDashboard,
  ShoppingBag,
  Package,
  Users,
  CircleEllipsis,
  type LucideIcon,
} from "lucide-react-native";
import { adminHaptics } from "@repo/mobile-shared/haptics/feedback";
import { Text } from "@/components/ui/Text";
import { theme } from "@/lib/theme";
import { DOCK_BOTTOM_GAP, DOCK_HEIGHT, useDockClearance } from "./dock-metrics";

export { DOCK_BOTTOM_GAP, DOCK_HEIGHT, useDockClearance };

// App-standard ease-out-quart — no bounce, no overshoot.
const ENTRANCE_EASING = Easing.bezier(0.22, 1, 0.36, 1);

const ICON_SIZE = 24;

// Inactive paper at 0.60 composites to ~#9A9996 on the ink bar (~6.8:1) —
// clears WCAG AA for the labels. Do NOT lower this: at 0.40 it drops to
// ~3.6:1 and fails.
const INACTIVE_TINT = "rgba(247, 246, 242, 0.60)";

const TAB_ICONS: Record<string, LucideIcon> = {
  index: LayoutDashboard,
  orders: ShoppingBag,
  products: Package,
  customers: Users,
  // CircleEllipsis, not MoreHorizontal: the bare three-dot glyph has only
  // ~9px of vertical ink in a 24pt box (vs. ~65px for the Orders/Products
  // glyphs), so its icon-to-label gap read as detached/misaligned even
  // though the label itself was correctly positioned. The circle ring
  // gives it comparable vertical mass to its siblings.
  more: CircleEllipsis,
};

// Short labels — slots are equal fifths, so a long title would truncate. The
// full screen title is still used for accessibility.
const TAB_LABELS: Record<string, string> = {
  index: "Home",
  orders: "Orders",
  products: "Products",
  customers: "Customers",
  more: "More",
};

// Minimal slice of the react-navigation tabBar props expo-router passes.
interface DockRoute {
  key: string;
  name: string;
}
interface DockProps {
  state: { index: number; routes: DockRoute[] };
  descriptors: Record<string, { options: { title?: string } }>;
  navigation: {
    emit: (event: {
      type: "tabPress";
      target: string;
      canPreventDefault: true;
    }) => { defaultPrevented: boolean };
    navigate: (name: string) => void;
  };
}

export function Dock({ state, descriptors, navigation }: DockProps) {
  const insets = useSafeAreaInsets();
  const reduceMotion = useReducedMotion();

  return (
    <View
      style={[styles.root, { bottom: insets.bottom + DOCK_BOTTOM_GAP }]}
      pointerEvents="box-none"
    >
      <View style={styles.bar} accessibilityRole="tablist">
        {state.routes.map((route, index) => {
          const { options } = descriptors[route.key] ?? { options: {} };
          const label = options?.title ?? route.name;
          const tabLabel = TAB_LABELS[route.name] ?? label;
          const isActive = state.index === index;
          const Icon = TAB_ICONS[route.name] ?? LayoutDashboard;

          const onPress = () => {
            const event = navigation.emit({
              type: "tabPress",
              target: route.key,
              canPreventDefault: true,
            });
            if (!isActive && !event.defaultPrevented) {
              void adminHaptics.selectionChanged();
              navigation.navigate(route.name);
            }
          };

          return (
            <Pressable
              key={route.key}
              onPress={onPress}
              accessibilityRole="tab"
              accessibilityState={{ selected: isActive }}
              accessibilityLabel={label}
              android_ripple={{ ...theme.press.rippleOnDark, borderless: true }}
              // STATIC style, not the `({pressed}) => …` callback form: under
              // NativeWind's JSX interop a function style prop isn't resolved
              // like a plain object, and `flex: 1` was being dropped — the five
              // slots sized to their content and packed left instead of taking
              // equal fifths. Tab switching is instant and haptic, so the iOS
              // press dim isn't worth reintroducing the risk for.
              style={styles.slot}
            >
              {isActive ? (
                <Animated.View
                  entering={
                    reduceMotion
                      ? undefined
                      : FadeIn.duration(220).easing(ENTRANCE_EASING)
                  }
                  style={styles.tabContent}
                >
                  {/* Weight + full opacity carry the active state. Filling a
                      lucide glyph muddies it — these are outline-designed and
                      have internal detail. A properly filled active icon needs
                      a real duotone set, which is its own task. */}
                  <Icon
                    size={ICON_SIZE}
                    color={theme.colors.inverse}
                    strokeWidth={2.4}
                  />
                  <Text
                    preset="caption"
                    className="font-sans-semibold"
                    style={styles.labelActive}
                    numberOfLines={1}
                  >
                    {tabLabel}
                  </Text>
                </Animated.View>
              ) : (
                <View style={styles.tabContent}>
                  <Icon
                    size={ICON_SIZE}
                    color={INACTIVE_TINT}
                    strokeWidth={1.8}
                  />
                  <Text
                    preset="caption"
                    className="font-sans-semibold"
                    style={styles.labelInactive}
                    numberOfLines={1}
                  >
                    {tabLabel}
                  </Text>
                </View>
              )}
            </Pressable>
          );
        })}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  root: {
    position: "absolute",
    left: 12,
    right: 12,
  },
  bar: {
    flexDirection: "row",
    alignItems: "center",
    height: DOCK_HEIGHT,
    borderRadius: 30,
    backgroundColor: theme.colors.text,
    paddingHorizontal: 8,
    shadowColor: "#000000",
    shadowOffset: { width: 0, height: 10 },
    shadowOpacity: 0.45,
    shadowRadius: 26,
    elevation: 12,
  },
  slot: {
    flex: 1,
    height: "100%",
    alignItems: "center",
    justifyContent: "center",
  },
  tabContent: {
    alignItems: "center",
    justifyContent: "center",
    gap: 3,
  },
  // 11pt is below the `caption` preset (13) on purpose — five labels have to
  // fit equal fifths of a 390pt bar without truncating.
  labelActive: {
    fontSize: 11,
    lineHeight: 14,
    color: theme.colors.inverse,
  },
  labelInactive: {
    fontSize: 11,
    lineHeight: 14,
    color: INACTIVE_TINT,
  },
});
