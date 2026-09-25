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
import { Fragment, createContext, useContext } from "react";
import { TextInput } from "react-native";

/**
 * The real package hands components an internal context and `BottomSheetTextInput`
 * registers its focus on it — that registration is what makes `keyboardBehavior`
 * move the sheet. FieldInput therefore decides which input to render by asking
 * `useBottomSheetInternal(true)` whether it is inside a sheet.
 *
 * A mock that always answered "no" would let FieldInput silently fall back to a
 * plain TextInput in every test, so the regression it guards against would be
 * invisible here — the shape this repo has been bitten by before. Provide a real
 * context instead, so a component rendered inside the mocked modal gets the same
 * answer it gets on a device.
 */
const InternalContext = createContext<object | null>(null);

export function useBottomSheetInternal(unsafe?: boolean) {
  const ctx = useContext(InternalContext);
  if (unsafe !== true && ctx === null) {
    throw "'useBottomSheetInternal' cannot be used out of the BottomSheet!";
  }
  return ctx;
}

/** Distinguishable from react-native's TextInput so a test can assert the swap. */
export function BottomSheetTextInput(props: Record<string, unknown>) {
  return <TextInput {...props} />;
}

/** BlockReasonSheet imports this; without it the backdrop renderer is undefined. */
export function BottomSheetBackdrop() {
  return null;
}

/**
 * `dismiss()` invokes the `onDismiss` prop, as the real modal does.
 *
 * Not cosmetic fidelity: several components own state that is released ONLY
 * by `onDismiss` (Orders' menu order, cancel target and refund target). A
 * mock whose `dismiss()` was a no-op left that state set for the whole test,
 * which is precisely the stale-target condition the screen has to defend
 * against — so the mock quietly guaranteed those bugs could not be
 * reproduced. `present()` stays a no-op: children render unconditionally
 * here, so there is nothing for it to do.
 */
export const BottomSheetModal = require("react").forwardRef(
  (
    { children, onDismiss }: { children?: React.ReactNode; onDismiss?: () => void },
    ref: React.Ref<unknown>,
  ) => {
    require("react").useImperativeHandle(
      ref,
      () => ({ present: () => {}, dismiss: () => onDismiss?.() }),
      [onDismiss],
    );
    return (
      <InternalContext.Provider value={{ mocked: true }}>
        {children ?? null}
      </InternalContext.Provider>
    );
  },
);

export function BottomSheetModalProvider({ children }: { children?: React.ReactNode }) {
  return children ?? null;
}

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
