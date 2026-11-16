import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";

vi.mock("next/navigation", () => ({ usePathname: () => "/sell", useRouter: () => ({ refresh: vi.fn(), push: vi.fn() }) }));

import { UserMenu } from "./UserMenu";

const open = () => fireEvent.click(screen.getByRole("button"));

describe("UserMenu", () => {
  it("links to Profile and Settings", () => {
    render(<UserMenu email="sam@example.com" />);
    open();
    expect(screen.getByRole("link", { name: /profile/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /settings/i })).toBeInTheDocument();
  });

  // A cashier is kept on the sale screen, so both would only lead back there.
  it("leaves them out for a cashier, keeping sign out", () => {
    render(<UserMenu email="sam@example.com" sellOnly />);
    open();
    expect(screen.queryByRole("link", { name: /profile/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /settings/i })).not.toBeInTheDocument();
    expect(screen.getByText(/sign out/i)).toBeInTheDocument();
  });
});
