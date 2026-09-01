import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";

import { AdminPage } from "./AdminPage";

// A `position: fixed` element is positioned against the VIEWPORT only while
// no ancestor establishes a containing block. Any ancestor with a transform
// takes that job over, and `inset-0` then covers that ancestor's box.
//
// AdminPage's entrance animation used `animation-fill-mode: both`, which
// keeps the ANIMATED value applied for as long as the page is mounted.
// fadeInUp ends on `transform: none`, but an animated `none` computes to
// the identity matrix(1, 0, 0, 1, 0, 0) — still a transform. Every modal
// rendered inside a page therefore drew its scrim as a grey rectangle over
// the content column, leaving the sidebar and header bright. Measured live:
// 1152x448 at (512, 113) in a 1920x779 viewport.
//
// `backwards` reverts to the natural style when the animation ends, which
// is visually identical here and leaves no containing block behind.
describe("AdminPage — entrance animation", () => {
  it("does not use fill-mode `both`, which would trap fixed-position children", () => {
    const { container } = render(
      <AdminPage title="Warehouses">
        <p>content</p>
      </AdminPage>,
    );

    const root = container.firstElementChild;
    expect(root).not.toBeNull();
    const cls = root!.className;

    expect(cls).toContain("animate-[fadeInUp");
    expect(cls).not.toMatch(/fadeInUp[^\]]*_both\]/);
    expect(cls).toMatch(/fadeInUp[^\]]*_backwards\]/);
  });

  it("still renders its children and title", () => {
    render(
      <AdminPage title="Warehouses">
        <p>content</p>
      </AdminPage>,
    );
    expect(screen.getByText("Warehouses")).toBeInTheDocument();
    expect(screen.getByText("content")).toBeInTheDocument();
  });
});
