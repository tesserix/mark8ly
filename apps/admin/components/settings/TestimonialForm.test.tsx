import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TestimonialForm } from "./TestimonialForm";

vi.mock("@/app/(admin)/settings/testimonial/actions", () => ({
  saveTestimonial: vi.fn(async () => ({ ok: true })),
}));
afterEach(cleanup);

it("starts with no rating or consent and lets merchants save privately", () => {
  render(<TestimonialForm storeName="My shop" initial={null} />);
  expect(screen.getByLabelText("Your rating")).toHaveValue("");
  expect(screen.getByRole("checkbox")).not.toBeChecked();
  expect(screen.getByRole("button", { name: "Publish review" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "Save privately" })).toBeEnabled();
});

it("requires fresh consent after editing the review or rating", () => {
  render(
    <TestimonialForm
      storeName="My shop"
      initial={{
        author_name: "Seller",
        quote: "My experience.",
        rating: 3,
        published: true,
      }}
    />,
  );
  expect(screen.getByLabelText("Your rating")).toHaveValue("3");
  const consent = screen.getByRole("checkbox");
  fireEvent.click(consent);
  expect(screen.getByRole("button", { name: "Publish review" })).toBeEnabled();
  fireEvent.change(screen.getByLabelText("Your review"), {
    target: { value: "An updated review." },
  });
  expect(consent).not.toBeChecked();
  fireEvent.click(consent);
  fireEvent.change(screen.getByLabelText("Your rating"), {
    target: { value: "2" },
  });
  expect(consent).not.toBeChecked();
  expect(
    screen.getByRole("button", { name: "Withdraw and save privately" }),
  ).toBeEnabled();
});
