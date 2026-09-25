// A text field inside a bottom sheet must be gorhom's own input.
//
// Reported from a device: the block-customer sheet renders behind the keyboard
// with no way to type. The sheet already sets `keyboardBehavior="interactive"`,
// but that only moves the sheet for an input gorhom knows about — it learns
// that from BottomSheetTextInput registering focus on the internal context. A
// plain react-native TextInput never registers, so the sheet stays put and the
// keyboard covers the field.
//
// Nine sheets render FieldInput, so this is asserted on the component itself
// rather than per sheet: the swap has to be automatic, because a per-sheet
// opt-in is one someone forgets on the tenth.

import { render } from "@testing-library/react-native";
import {
  BottomSheetModal,
  BottomSheetTextInput,
} from "@gorhom/bottom-sheet";
import { TextInput } from "react-native";
import { FieldInput } from "@/components/ui/FieldInput";

describe("FieldInput picks the input its surroundings require", () => {
  it("uses gorhom's BottomSheetTextInput inside a sheet", () => {
    const { UNSAFE_queryAllByType } = render(
      <BottomSheetModal>
        <FieldInput label="Reason" placeholder="Why are you blocking them?" />
      </BottomSheetModal>,
    );

    // Without the swap this is 0 and the plain TextInput below is 1 — the
    // device bug exactly, and the assertion that fails without the fix.
    expect(UNSAFE_queryAllByType(BottomSheetTextInput)).toHaveLength(1);
  });

  it("uses a plain TextInput on an ordinary screen", () => {
    const { UNSAFE_queryAllByType } = render(
      <FieldInput label="Title" placeholder="Product title" />,
    );

    // The unsafe overload of useBottomSheetInternal must return null rather
    // than throw here — FieldInput is used on ordinary form screens too, and
    // a throw would take out every product form.
    expect(UNSAFE_queryAllByType(BottomSheetTextInput)).toHaveLength(0);
    expect(UNSAFE_queryAllByType(TextInput).length).toBeGreaterThanOrEqual(1);
  });
});
