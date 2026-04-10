import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { MediaGrid } from "./MediaGrid";
import type { AdminMediaResponse } from "@/lib/api/marketplace-api";

// Capture onDragEnd so we can trigger the handler directly from tests.
let capturedOnDragEnd: ((e: unknown) => void) | null = null;
vi.mock("@dnd-kit/core", async () => {
  const actual = await vi.importActual<Record<string, unknown>>("@dnd-kit/core");
  return {
    ...actual,
    DndContext: ({ children, onDragEnd }: { children: ReactNode; onDragEnd: (e: unknown) => void }) => {
      capturedOnDragEnd = onDragEnd;
      return <div>{children}</div>;
    },
  };
});

function makeMedia(id: string, position: number, alt: string): AdminMediaResponse {
  return {
    id,
    url: `https://cdn.test/${id}.jpg`,
    storage_key: `products/${id}`,
    alt,
    position,
    media_type: "image",
    variant_id: null,
    width: 800,
    height: 800,
    bytes: 1024,
  };
}

describe("MediaGrid", () => {
  const items = [
    makeMedia("a", 0, "Front view"),
    makeMedia("b", 1, "Side view"),
    makeMedia("c", 2, "Back view"),
  ];

  it("renders one card per item with alt text as accessible name", () => {
    render(<MediaGrid items={items} onReorder={vi.fn()} onAction={vi.fn()} />);
    expect(screen.getByAltText("Front view")).toBeInTheDocument();
    expect(screen.getByAltText("Side view")).toBeInTheDocument();
    expect(screen.getByAltText("Back view")).toBeInTheDocument();
  });

  it("marks the first item as primary", () => {
    const { container } = render(
      <MediaGrid items={items} onReorder={vi.fn()} onAction={vi.fn()} />,
    );
    const primary = container.querySelectorAll('[data-primary="true"]');
    expect(primary).toHaveLength(1);
    const firstPrimary = primary[0];
    expect(firstPrimary).toBeTruthy();
    expect(within(firstPrimary as HTMLElement).getByAltText("Front view")).toBeInTheDocument();
  });

  it("calls onAction with delete when Delete menu item clicked", () => {
    const onAction = vi.fn();
    render(<MediaGrid items={items} onReorder={vi.fn()} onAction={onAction} />);
    const buttons = screen.getAllByRole("button", { name: /image actions/i });
    fireEvent.click(buttons[0]!);
    fireEvent.click(screen.getByRole("menuitem", { name: /delete/i }));
    expect(onAction).toHaveBeenCalledWith("delete", items[0]);
  });

  it("drag end reorders when active and over differ; no-op branches return early", () => {
    const onReorder = vi.fn();
    render(<MediaGrid items={items} onReorder={onReorder} onAction={vi.fn()} />);
    // Real reorder: move "a" onto "c"
    capturedOnDragEnd?.({ active: { id: "a" }, over: { id: "c" } });
    expect(onReorder).toHaveBeenCalledTimes(1);
    expect(onReorder.mock.calls[0]![0].map((m: AdminMediaResponse) => m.id)).toEqual(["b", "c", "a"]);
    // Same id → early return
    capturedOnDragEnd?.({ active: { id: "a" }, over: { id: "a" } });
    // over null → early return
    capturedOnDragEnd?.({ active: { id: "a" }, over: null });
    // unknown id → findIndex < 0 branch
    capturedOnDragEnd?.({ active: { id: "zzz" }, over: { id: "a" } });
    expect(onReorder).toHaveBeenCalledTimes(1);
  });
});
