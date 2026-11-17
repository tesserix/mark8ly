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
  expect(screen.getByRole("checkbox", { name: /honest review/ })).not.toBeChecked();
  expect(screen.getByRole("checkbox", { name: "Show my name on the review" })).not.toBeChecked();
  expect(screen.queryByLabelText("Your name")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Publish review" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "Save privately" })).toBeEnabled();
});

it("requires fresh consent after editing the review or rating", () => {
  render(
    <TestimonialForm
      storeName="My shop"
      initial={{
        position: "Store owner",
        display_name: null,
        quote: "My experience.",
        rating: 3,
        published: true,
      }}
    />,
  );
  expect(screen.getByLabelText("Your rating")).toHaveValue("3");
  const consent = screen.getByRole("checkbox", { name: /honest review/ });
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

it("shows the name field only when the merchant opts in, and asks for fresh consent", () => {
  render(
    <TestimonialForm
      storeName="My shop"
      initial={{
        position: "Store owner",
        display_name: "Sam",
        quote: "My experience.",
        rating: 5,
        published: true,
      }}
    />,
  );
  const showName = screen.getByRole("checkbox", { name: "Show my name on the review" });
  expect(showName).toBeChecked();
  expect(screen.getByLabelText("Your name")).toHaveValue("Sam");
  const consent = screen.getByRole("checkbox", { name: /honest review/ });
  fireEvent.click(consent);
  fireEvent.click(showName);
  expect(screen.queryByLabelText("Your name")).not.toBeInTheDocument();
  expect(consent).not.toBeChecked();
});
