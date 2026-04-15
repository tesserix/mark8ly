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
          storeId="test-store"
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
          storeId="test-store"
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
          storeId="test-store"
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
          storeId="test-store"
        />
      );
    });

    // Valid because alt is filled
    expect(onValidityChange).not.toHaveBeenCalledWith(false);
    expect(onValidityChange).toHaveBeenCalledWith(true);
    expect(screen.queryByText(/alt text is required/i)).not.toBeInTheDocument();
  });

  it("secondary CTA label set, URL empty — onValidityChange(false)", async () => {
    const onValidityChange = vi.fn();
    await act(async () => {
      render(
        <HeroEditor
          value={baseHero({
            cta_secondary_label: "Read the story",
            cta_secondary_url: null,
          })}
          onChange={() => {}}
          onValidityChange={onValidityChange}
          pages={[]}
          editable={true}
          storeId="test-store"
        />
      );
    });
    expect(onValidityChange).toHaveBeenCalledWith(false);
  });

  it("secondary CTA URL set, label empty — onValidityChange(false)", async () => {
    const onValidityChange = vi.fn();
    await act(async () => {
      render(
        <HeroEditor
          value={baseHero({
            cta_secondary_label: null,
            cta_secondary_url: "/pages/about",
          })}
          onChange={() => {}}
          onValidityChange={onValidityChange}
          pages={[]}
          editable={true}
          storeId="test-store"
        />
      );
    });
    expect(onValidityChange).toHaveBeenCalledWith(false);
  });

  it("secondary CTA both set — onValidityChange(true)", async () => {
    const onValidityChange = vi.fn();
    await act(async () => {
      render(
        <HeroEditor
          value={baseHero({
            cta_secondary_label: "Read the story",
            cta_secondary_url: "/pages/about",
          })}
          onChange={() => {}}
          onValidityChange={onValidityChange}
          pages={[]}
          editable={true}
          storeId="test-store"
        />
      );
    });
    expect(onValidityChange).not.toHaveBeenCalledWith(false);
    expect(onValidityChange).toHaveBeenCalledWith(true);
  });

  it("secondary CTA both empty — onValidityChange(true)", async () => {
    const onValidityChange = vi.fn();
    await act(async () => {
      render(
        <HeroEditor
          value={baseHero()}
          onChange={() => {}}
          onValidityChange={onValidityChange}
          pages={[]}
          editable={true}
          storeId="test-store"
        />
      );
    });
    expect(onValidityChange).not.toHaveBeenCalledWith(false);
  });

  it("aside URL cleared after being set — error clears, onValidityChange(true)", async () => {
    const onValidityChange = vi.fn();
    const onChange = vi.fn();

    const { rerender } = render(
      <HeroEditor
        value={baseHero({
          aside_image_url: "https://cdn.example.com/aside.jpg",
          aside_image_alt: null,
        })}
        onChange={onChange}
        onValidityChange={onValidityChange}
        pages={[]}
        editable={true}
        storeId="test-store"
      />
    );

    // Confirm the error is initially visible (URL set, no alt)
    expect(
      screen.getByText(/alt text is required when an aside image is set/i)
    ).toBeInTheDocument();

    // Parent clears the aside URL (via the image-upload "Remove" button)
    // and the component re-renders with it unset.
    await act(async () => {
      rerender(
        <HeroEditor
          value={baseHero({ aside_image_url: null, aside_image_alt: null })}
          onChange={onChange}
          onValidityChange={onValidityChange}
          pages={[]}
          editable={true}
          storeId="test-store"
        />
      );
    });

    expect(screen.queryByText(/alt text is required/i)).not.toBeInTheDocument();
    const calls = onValidityChange.mock.calls;
    const lastCall = calls[calls.length - 1];
    expect(lastCall?.[0]).toBe(true);
  });
});
