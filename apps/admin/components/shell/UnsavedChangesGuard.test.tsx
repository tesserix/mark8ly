import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

const push = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push }) }));

import {
  UnsavedChangesProvider,
  useUnsavedNavigationGuard,
} from "./UnsavedChangesGuard";

/** A form that reports itself dirty, exactly as useUnsavedGuard does. */
function DirtyForm({ dirty = true, submitting = false }) {
  useUnsavedNavigationGuard("form-1", dirty, submitting);
  return <p>form</p>;
}

function renderWithLinks(form: React.ReactNode, links: React.ReactNode) {
  return render(
    <UnsavedChangesProvider>
      {form}
      {links}
    </UnsavedChangesProvider>,
  );
}

const dialog = () => screen.queryByText(/leave without saving\?/i);

beforeEach(() => {
  vi.clearAllMocks();
  window.history.pushState({}, "", "/products/p1");
});

// App Router navigation fires no beforeunload, so a sidebar link with a dirty
// form used to unmount it and discard the edits with no warning at all. That
// is the silent half this guard exists to close — the loud half (refresh, tab
// close) stays with beforeunload, whose dialog the browser owns.
describe("UnsavedChangesGuard", () => {
  it("intercepts an in-app link while a form is dirty", () => {
    renderWithLinks(<DirtyForm />, <a href="/orders">Orders</a>);
    fireEvent.click(screen.getByText("Orders"));
    expect(dialog()).toBeInTheDocument();
    expect(push).not.toHaveBeenCalled();
  });

  it("navigates once the merchant confirms", () => {
    renderWithLinks(<DirtyForm />, <a href="/orders">Orders</a>);
    fireEvent.click(screen.getByText("Orders"));
    fireEvent.click(screen.getByRole("button", { name: /^leave$/i }));
    expect(push).toHaveBeenCalledWith("/orders");
  });

  it("stays put when they decline, and can still be asked again", () => {
    renderWithLinks(<DirtyForm />, <a href="/orders">Orders</a>);
    fireEvent.click(screen.getByText("Orders"));
    fireEvent.click(screen.getByRole("button", { name: /stay on this page/i }));
    expect(push).not.toHaveBeenCalled();
    expect(dialog()).not.toBeInTheDocument();

    // The guard must not disarm itself by asking once.
    fireEvent.click(screen.getByText("Orders"));
    expect(dialog()).toBeInTheDocument();
  });

  it("lets the click through when nothing is dirty", () => {
    renderWithLinks(<DirtyForm dirty={false} />, <a href="/orders">Orders</a>);
    fireEvent.click(screen.getByText("Orders"));
    expect(dialog()).not.toBeInTheDocument();
  });

  it("lets the click through while the form is submitting", () => {
    // A save in flight is about to navigate on purpose; asking there would
    // interrupt the very action the merchant took.
    renderWithLinks(<DirtyForm submitting />, <a href="/orders">Orders</a>);
    fireEvent.click(screen.getByText("Orders"));
    expect(dialog()).not.toBeInTheDocument();
  });

  it("releases the form's claim when it unmounts", () => {
    const { rerender } = renderWithLinks(<DirtyForm />, <a href="/orders">Orders</a>);
    rerender(
      <UnsavedChangesProvider>
        <a href="/orders">Orders</a>
      </UnsavedChangesProvider>,
    );
    fireEvent.click(screen.getByText("Orders"));
    expect(dialog()).not.toBeInTheDocument();
  });

  // Each of these unmounts nothing, or is already covered by beforeunload.
  describe("clicks it must not intercept", () => {
    it("a new-tab link", () => {
      renderWithLinks(
        <DirtyForm />,
        <a href="/orders" target="_blank" rel="noreferrer">Orders</a>,
      );
      fireEvent.click(screen.getByText("Orders"));
      expect(dialog()).not.toBeInTheDocument();
    });

    it("a cmd/ctrl click", () => {
      renderWithLinks(<DirtyForm />, <a href="/orders">Orders</a>);
      fireEvent.click(screen.getByText("Orders"), { metaKey: true });
      expect(dialog()).not.toBeInTheDocument();
    });

    it("an external link, which beforeunload already covers", () => {
      renderWithLinks(
        <DirtyForm />,
        <a href="https://example.com/x">Away</a>,
      );
      fireEvent.click(screen.getByText("Away"));
      expect(dialog()).not.toBeInTheDocument();
    });

    it("an in-page anchor", () => {
      renderWithLinks(<DirtyForm />, <a href="#media">Media</a>);
      fireEvent.click(screen.getByText("Media"));
      expect(dialog()).not.toBeInTheDocument();
    });

    it("a download link", () => {
      renderWithLinks(<DirtyForm />, <a href="/invoice.pdf" download>Invoice</a>);
      fireEvent.click(screen.getByText("Invoice"));
      expect(dialog()).not.toBeInTheDocument();
    });

    it("a link back to the page already open", () => {
      renderWithLinks(<DirtyForm />, <a href="/products/p1">This product</a>);
      fireEvent.click(screen.getByText("This product"));
      expect(dialog()).not.toBeInTheDocument();
    });

    it("an anchor that owns its own confirmation", () => {
      // ProductForm's Discard already opens a styled dialog of its own.
      renderWithLinks(
        <DirtyForm />,
        <a href="/products" data-unsaved-guard="off">Discard</a>,
      );
      fireEvent.click(screen.getByText("Discard"));
      expect(dialog()).not.toBeInTheDocument();
    });

    it("a plain button", () => {
      renderWithLinks(<DirtyForm />, <button type="button">Save</button>);
      fireEvent.click(screen.getByText("Save"));
      expect(dialog()).not.toBeInTheDocument();
    });
  });
});
