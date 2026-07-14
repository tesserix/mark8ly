// Dock — floating bottom navigation for mark8ly admin (replaces the flush
// tab bar). A detached rounded bar hovering above the home indicator: solid
// Paper surface, hairline border, soft single-elevation shadow — no blur/glass
// (per the editorial system). Inactive tabs are icon-only; the active tab
// expands into a solid Ink pill with a Paper label (the dock's single accent).
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
  MoreHorizontal,
  type LucideIcon,
} from "lucide-react-native";
import { Text } from "@/components/ui/Text";
import { theme } from "@/lib/theme";
import { DOCK_BOTTOM_GAP, DOCK_HEIGHT, useDockClearance } from "./dock-metrics";

export { DOCK_BOTTOM_GAP, DOCK_HEIGHT, useDockClearance };

// App-standard ease-out-quart — no bounce, no overshoot.
const ENTRANCE_EASING = Easing.bezier(0.22, 1, 0.36, 1);

const TAB_ICONS: Record<string, LucideIcon> = {
  index: LayoutDashboard,
  orders: ShoppingBag,
  products: Package,
  customers: Users,
  more: MoreHorizontal,
};

// Short labels for the active pill — slots are equal fifths, so a long title
// would truncate. The full screen title is still used for accessibility.
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
          const pillLabel = TAB_LABELS[route.name] ?? label;
          const isActive = state.index === index;
          const Icon = TAB_ICONS[route.name] ?? LayoutDashboard;

          const onPress = () => {
            const event = navigation.emit({
              type: "tabPress",
              target: route.key,
              canPreventDefault: true,
            });
            if (!isActive && !event.defaultPrevented) {
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
              style={styles.slot}
            >
              {isActive ? (
                <Animated.View
                  entering={
                    reduceMotion
                      ? undefined
                      : FadeIn.duration(220).easing(ENTRANCE_EASING)
                  }
                  style={styles.activePill}
                >
                  <Icon size={19} color={theme.colors.inverse} strokeWidth={2} />
                  <Text
                    preset="caption"
                    color="inverse"
                    className="font-sans-semibold"
                    numberOfLines={1}
                  >
                    {pillLabel}
                  </Text>
                </Animated.View>
              ) : (
                <Icon
                  size={22}
                  color={theme.colors.textTertiary}
                  strokeWidth={1.9}
                />
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
    borderRadius: 26,
    backgroundColor: theme.colors.elevated,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: theme.colors.border,
    paddingHorizontal: 6,
    shadowColor: "#000000",
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.1,
    shadowRadius: 16,
    elevation: 10,
  },
  slot: {
    flex: 1,
    height: "100%",
    alignItems: "center",
    justifyContent: "center",
  },
  activePill: {
    flexDirection: "row",
    alignItems: "center",
    gap: 5,
    paddingHorizontal: 12,
    paddingVertical: 9,
    borderRadius: 18,
    backgroundColor: theme.colors.text,
  },
});
