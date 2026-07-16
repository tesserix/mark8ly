// Shared @gorhom/bottom-sheet jest mock for tests that need to drive a real
// selection flow through CategoryPickerSheet (unlike category-field.test.tsx,
// which only exercises field-level states and never needs a rendered row).
//
// Kept in its own module — rather than inlined in a jest.mock() factory —
// because nativewind's babel transform instruments JSX/createElement calls
// with a `_ReactNativeCSSInterop` reference; jest.mock() factories are
// hoisted above that reference's declaration, so JSX written directly inside
// a factory throws "module factory is not allowed to reference any
// out-of-scope variables". A plain required module has no such hoisting
// constraint.
import { Fragment } from "react";

export const BottomSheetModal = require("react").forwardRef(
  ({ children }: { children?: React.ReactNode }, ref: React.Ref<unknown>) => {
    require("react").useImperativeHandle(ref, () => ({ present: () => {}, dismiss: () => {} }));
    return children ?? null;
  },
);

export function BottomSheetView({ children }: { children?: React.ReactNode }) {
  return children ?? null;
}

export function BottomSheetScrollView({ children }: { children?: React.ReactNode }) {
  return children ?? null;
}

/**
 * Maps `data` through `renderItem` like a real FlatList — CategoryPickerSheet
 * renders its tree rows via `data`/`renderItem` props, not `children`.
 */
export function BottomSheetFlatList<T>({
  data,
  renderItem,
  keyExtractor,
}: {
  data: T[];
  renderItem: (info: { item: T }) => React.ReactNode;
  keyExtractor: (item: T) => string;
}) {
  return (
    <>
      {data.map((item) => (
        <Fragment key={keyExtractor(item)}>{renderItem({ item })}</Fragment>
      ))}
    </>
  );
}
