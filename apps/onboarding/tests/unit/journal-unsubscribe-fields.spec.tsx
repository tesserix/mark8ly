import { expect, test } from "@playwright/test";
import path from "node:path";
import { createElement, type ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";

import { loadTsxExport } from "./helpers/load-tsx";
import type { JournalUnsubscribeStatus } from "../../components/marketing/JournalUnsubscribeFields";

/**
 * See tests/unit/helpers/load-tsx.ts for why JournalUnsubscribeFields'
 * real JSX source is loaded this way rather than imported directly.
 */
type JournalUnsubscribeFieldsProps = {
  status: JournalUnsubscribeStatus;
  errorMessage: string | null;
  onConfirm: () => void;
};

const JournalUnsubscribeFields = loadTsxExport<
  (props: JournalUnsubscribeFieldsProps) => ReactNode
>(
  path.join(
    __dirname,
    "../../components/marketing/JournalUnsubscribeFields.tsx",
  ),
  "JournalUnsubscribeFields",
  __dirname,
);

function noop() {
  // Static rendering never fires onConfirm.
}

test("idle state renders a Confirm button that is not disabled", () => {
  const html = renderToStaticMarkup(
    createElement(JournalUnsubscribeFields, {
      status: "idle",
      errorMessage: null,
      onConfirm: noop,
    }),
  );

  expect(html).toContain("<button");
  expect(html).not.toContain("disabled=");
  expect(html.toLowerCase()).toContain("unsubscribe");
});

test("idle state renders no error and no status region", () => {
  const html = renderToStaticMarkup(
    createElement(JournalUnsubscribeFields, {
      status: "idle",
      errorMessage: null,
      onConfirm: noop,
    }),
  );

  expect(html).not.toContain('role="alert"');
  expect(html).not.toContain('role="status"');
});

test("submitting state disables the button and shows an in-progress label", () => {
  const html = renderToStaticMarkup(
    createElement(JournalUnsubscribeFields, {
      status: "submitting",
      errorMessage: null,
      onConfirm: noop,
    }),
  );

  expect(html).toContain('disabled=""');
});

test("done state announces success via role=status and disables the button", () => {
  const html = renderToStaticMarkup(
    createElement(JournalUnsubscribeFields, {
      status: "done",
      errorMessage: null,
      onConfirm: noop,
    }),
  );

  expect(html).toContain('role="status"');
  expect(html).toContain('disabled=""');
});

test("error state announces the message via role=alert and re-enables the button", () => {
  const html = renderToStaticMarkup(
    createElement(JournalUnsubscribeFields, {
      status: "error",
      errorMessage: "Something went wrong. Please try again.",
      onConfirm: noop,
    }),
  );

  expect(html).toContain('role="alert"');
  expect(html).toContain("Something went wrong. Please try again.");
  expect(html).not.toContain('disabled=""');
});
