import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { VariantImagePicker } from "./VariantImagePicker";
import type { AdminMediaResponse } from "@/lib/api/marketplace-api";

function makeMedia(id: string, alt: string): AdminMediaResponse {
  return {
    id,
    url: `https://cdn.test/${id}.jpg`,
    storage_key: `products/${id}`,
    alt,
    position: 0,
    media_type: "image",
    variant_id: null,
    width: 800,
    height: 800,
    bytes: 1024,
  };
}

describe("VariantImagePicker", () => {
  const media = [makeMedia("a", "Front"), makeMedia("b", "Back")];

  it("emits selected id via onSelect", () => {
    const onSelect = vi.fn();
    render(<VariantImagePicker media={media} currentMediaId={null} onSelect={onSelect} />);
    fireEvent.click(screen.getByRole("radio", { name: /front/i }));
    expect(onSelect).toHaveBeenCalledWith("a");
  });

  it("emits null on Remove image", () => {
    const onSelect = vi.fn();
    render(<VariantImagePicker media={media} currentMediaId="a" onSelect={onSelect} />);
    fireEvent.click(screen.getByRole("button", { name: /remove image/i }));
    expect(onSelect).toHaveBeenCalledWith(null);
  });
});
