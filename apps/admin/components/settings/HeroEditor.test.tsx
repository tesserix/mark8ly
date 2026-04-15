import { describe, it, expect, vi } from "vitest";
import { render, screen, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HeroEditor } from "./HeroEditor";
import type { HomepageHero } from "@/lib/api/marketplace-api";

function baseHero(overrides?: Partial<HomepageHero>): HomepageHero {
  return {
    enabled: true,
    heading: "Test heading",
    subheading: null,
    image_url: null,
    cta_label: null,
    cta_url: null,
    eyebrow: null,
    cta_secondary_label: null,
    cta_secondary_url: null,
    aside_image_url: null,
    aside_image_alt: null,
    ...overrides,
  };
}

describe("HeroEditor — aside image a11y validity", () => {
  it("default (no aside URL, no alt) — onValidityChange called with true, no error shown", async () => {
    const onValidityChange = vi.fn();
    render(
      <HeroEditor
        value={baseHero()}
        onChange={() => {}}
        onValidityChange={onValidityChange}
        pages={[]}
        editable={true}
      />
    );

    // useEffect fires synchronously in jsdom with act wrapping from render
    expect(onValidityChange).not.toHaveBeenCalledWith(false);
    expect(screen.queryByText(/alt text is required/i)).not.toBeInTheDocument();
  });

  it("aside URL set + alt empty — onValidityChange(false) + error visible", async () => {
    const onValidityChange = vi.fn();
    const hero = baseHero({ aside_image_url: null, aside_image_alt: null });
    const { rerender } = render(
      <HeroEditor
        value={hero}
        onChange={() => {}}
        onValidityChange={onValidityChange}
        pages={[]}
        editable={true}
      />
    );

    // Simulate parent updating value with aside URL but no alt
    const withUrl = baseHero({
      aside_image_url: "https://cdn.example.com/aside.jpg",
      aside_image_alt: null,
    });

    await act(async () => {
      rerender(
        <HeroEditor
          value={withUrl}
          onChange={() => {}}
          onValidityChange={onValidityChange}
          pages={[]}
          editable={true}
        />
      );
    });

    expect(onValidityChange).toHaveBeenCalledWith(false);
    expect(
      screen.getByText(/alt text is required when an aside image is set/i)
    ).toBeInTheDocument();
  });

  it("aside URL set + alt filled — onValidityChange(true), no error", async () => {
    const onValidityChange = vi.fn();

    const withBoth = baseHero({
      aside_image_url: "https://cdn.example.com/aside.jpg",
      aside_image_alt: "A model wearing the signature jacket",
    });

    await act(async () => {
      render(
        <HeroEditor
          value={withBoth}
          onChange={() => {}}
          onValidityChange={onValidityChange}
          pages={[]}
          editable={true}
        />
      );
    });

    // Valid because alt is filled
    expect(onValidityChange).not.toHaveBeenCalledWith(false);
    expect(onValidityChange).toHaveBeenCalledWith(true);
    expect(screen.queryByText(/alt text is required/i)).not.toBeInTheDocument();
  });

  it("aside URL cleared after being set — error clears, onValidityChange(true)", async () => {
    const user = userEvent.setup();
    const onValidityChange = vi.fn();

    let currentValue = baseHero({
      aside_image_url: "https://cdn.example.com/aside.jpg",
      aside_image_alt: null,
    });

    const onChange = vi.fn((next: HomepageHero) => {
      currentValue = next;
    });

    const { rerender } = render(
      <HeroEditor
        value={currentValue}
        onChange={onChange}
        onValidityChange={onValidityChange}
        pages={[]}
        editable={true}
      />
    );

    // Confirm the error is initially visible (URL set, no alt)
    expect(
      screen.getByText(/alt text is required when an aside image is set/i)
    ).toBeInTheDocument();

    // Clear the aside URL input by its stable id
    const asideUrlById = document.getElementById("hero_aside_image_url") as HTMLInputElement;
    expect(asideUrlById).not.toBeNull();

    await user.clear(asideUrlById);

    // Simulate parent receiving the cleared value and passing it back
    await act(async () => {
      rerender(
        <HeroEditor
          value={baseHero({ aside_image_url: null, aside_image_alt: null })}
          onChange={onChange}
          onValidityChange={onValidityChange}
          pages={[]}
          editable={true}
        />
      );
    });

    expect(screen.queryByText(/alt text is required/i)).not.toBeInTheDocument();
    // Last call should be true (valid)
    const calls = onValidityChange.mock.calls;
    const lastCall = calls[calls.length - 1];
    expect(lastCall?.[0]).toBe(true);
  });
});
