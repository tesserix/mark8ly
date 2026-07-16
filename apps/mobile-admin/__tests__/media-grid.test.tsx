// MediaGrid imports icons from lucide-react-native's ESM build
// (`dist/esm/...mjs`) — not covered by jest-expo's default
// transformIgnorePatterns, so requiring it unmocked throws "Unexpected token
// 'export'". Stub every icon export with a no-op component; we don't assert
// on icon rendering here. (Pattern lifted verbatim from security.test.tsx.)
jest.mock("lucide-react-native", () => {
  const IconStub = () => null;
  return new Proxy({}, { get: () => IconStub });
});

import { render, fireEvent } from "@testing-library/react-native";
import { MediaGrid, sortMedia } from "@/components/products/MediaGrid";

const media = (id: string, position: number, alt?: string) => ({
  id,
  url: `https://cdn.mark8ly.com/${id}.png`,
  storage_key: `tenants/x/${id}.png`,
  position,
  media_type: "image",
  ...(alt ? { alt } : {}),
});

const UNSORTED = [media("c", 2), media("a", 0), media("b", 1)];

describe("sortMedia", () => {
  it("orders by position — the wire does not guarantee array order", () => {
    expect(sortMedia(UNSORTED as never).map((m) => m.id)).toEqual(["a", "b", "c"]);
  });

  it("does not mutate its input", () => {
    const input = [...UNSORTED];
    sortMedia(input as never);
    expect(input.map((m) => m.id)).toEqual(["c", "a", "b"]);
  });
});

describe("MediaGrid", () => {
  const noop = () => {};

  it("marks the position-0 photo as the hero", () => {
    const { getByLabelText } = render(
      <MediaGrid
        media={UNSORTED as never}
        onReorder={noop}
        onAltChange={noop}
        onPress={noop}
        onLongPress={noop}
      />,
    );
    expect(getByLabelText("Photo 1, main image")).toBeTruthy();
  });

  it("moving a photo left emits its new position", () => {
    const onReorder = jest.fn();
    const { getByLabelText } = render(
      <MediaGrid
        media={UNSORTED as never}
        onReorder={onReorder}
        onAltChange={noop}
        onPress={noop}
        onLongPress={noop}
      />,
    );
    fireEvent.press(getByLabelText("Move photo 2 earlier"));
    expect(onReorder).toHaveBeenCalledWith("b", 0);
  });

  it("the first photo cannot move earlier", () => {
    const { queryByLabelText } = render(
      <MediaGrid
        media={UNSORTED as never}
        onReorder={noop}
        onAltChange={noop}
        onPress={noop}
        onLongPress={noop}
      />,
    );
    expect(queryByLabelText("Move photo 1 earlier")).toBeNull();
  });

  it("the last photo cannot move later", () => {
    const { queryByLabelText } = render(
      <MediaGrid
        media={UNSORTED as never}
        onReorder={noop}
        onAltChange={noop}
        onPress={noop}
        onLongPress={noop}
      />,
    );
    expect(queryByLabelText("Move photo 3 later")).toBeNull();
  });

  it("commits alt text on blur", () => {
    const onAltChange = jest.fn();
    const { getByLabelText } = render(
      <MediaGrid
        media={[media("a", 0)] as never}
        onReorder={noop}
        onAltChange={onAltChange}
        onPress={noop}
        onLongPress={noop}
      />,
    );
    const input = getByLabelText("Alt text for photo 1");
    fireEvent.changeText(input, "Linen robe, front");
    fireEvent(input, "blur");
    expect(onAltChange).toHaveBeenCalledWith("a", "Linen robe, front");
  });

  it("does not re-commit unchanged alt text", () => {
    const onAltChange = jest.fn();
    const { getByLabelText } = render(
      <MediaGrid
        media={[media("a", 0, "Existing")] as never}
        onReorder={noop}
        onAltChange={onAltChange}
        onPress={noop}
        onLongPress={noop}
      />,
    );
    fireEvent(getByLabelText("Alt text for photo 1"), "blur");
    expect(onAltChange).not.toHaveBeenCalled();
  });
});
