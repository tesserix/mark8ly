import { expect, test } from "@playwright/test";
import path from "node:path";
import { createElement, type ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";

import { loadTsxExport } from "./helpers/load-tsx";
import type { JournalSignupStatus } from "../../components/marketing/JournalSignupFields";

/**
 * See tests/unit/helpers/load-tsx.ts for why JournalSignupFields' real
 * JSX source is loaded this way rather than imported directly.
 */
type JournalSignupFieldsProps = {
  status: JournalSignupStatus;
  email: string;
  errorMessage: string | null;
  onEmailChange: (value: string) => void;
};

const JournalSignupFields = loadTsxExport<(props: JournalSignupFieldsProps) => ReactNode>(
  path.join(__dirname, "../../components/marketing/JournalSignupFields.tsx"),
  "JournalSignupFields",
  __dirname,
);

function noop() {
  // JournalSignupFields requires onEmailChange; static rendering never
  // fires it.
}

test("the email input has a real <label> associated via htmlFor/id", () => {
  const html = renderToStaticMarkup(
    createElement(JournalSignupFields, {
      status: "idle",
      email: "",
      errorMessage: null,
      onEmailChange: noop,
    }),
  );

  expect(html).toContain('for="journal-email"');
  expect(html).toContain('id="journal-email"');
  expect(html).toContain('type="email"');
});

test("idle state renders no error and no aria-describedby", () => {
  const html = renderToStaticMarkup(
    createElement(JournalSignupFields, {
      status: "idle",
      email: "",
      errorMessage: null,
      onEmailChange: noop,
    }),
  );

  expect(html).not.toContain("aria-describedby");
  expect(html).not.toContain('role="alert"');
});

test("error state wires aria-describedby on the input to the error message's id", () => {
  const html = renderToStaticMarkup(
    createElement(JournalSignupFields, {
      status: "error",
      email: "not-an-email",
      errorMessage: "That doesn't look like a valid email address.",
      onEmailChange: noop,
    }),
  );

  expect(html).toContain('aria-describedby="journal-email-error"');
  expect(html).toContain('id="journal-email-error"');
  expect(html).toContain('role="alert"');
  expect(html).toContain("That doesn&#x27;t look like a valid email address.");
  // The input itself must be marked invalid for assistive tech.
  expect(html).toContain('aria-invalid="true"');
});

test("success state announces a calm, specific message via role=status", () => {
  const html = renderToStaticMarkup(
    createElement(JournalSignupFields, {
      status: "success",
      email: "ada@example.com",
      errorMessage: null,
      onEmailChange: noop,
    }),
  );

  expect(html).toContain('role="status"');
  expect(html).toContain("we’ll email you when the first piece goes up");
});

test("submitting and success states disable the input", () => {
  const submitting = renderToStaticMarkup(
    createElement(JournalSignupFields, {
      status: "submitting",
      email: "ada@example.com",
      errorMessage: null,
      onEmailChange: noop,
    }),
  );
  const success = renderToStaticMarkup(
    createElement(JournalSignupFields, {
      status: "success",
      email: "ada@example.com",
      errorMessage: null,
      onEmailChange: noop,
    }),
  );

  expect(submitting).toContain('disabled=""');
  expect(success).toContain('disabled=""');
});
