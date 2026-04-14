import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";

import { Markdown } from "./markdown";

describe("Markdown", () => {
  it("renders plain markdown as HTML", () => {
    const { container } = render(<Markdown>{"# Hello\n\n**bold** text"}</Markdown>);
    expect(container.querySelector("h1")?.textContent).toBe("Hello");
    expect(container.querySelector("strong")?.textContent).toBe("bold");
  });

  it("strips raw <script> tags (XSS guard via skipHtml)", () => {
    const body = 'before<script>window.__xss=1</script>after';
    const { container } = render(<Markdown>{body}</Markdown>);
    // The script tag must NOT land in the DOM — no script element means
    // the browser has nothing to execute. skipHtml does keep the script's
    // text content rendered as plain text (inert), which is acceptable:
    // no DOM <script> node => no JS ever runs.
    expect(container.querySelector("script")).toBeNull();
  });

  it("strips raw <iframe> tags", () => {
    const body = "before<iframe src='https://evil.example'></iframe>after";
    const { container } = render(<Markdown>{body}</Markdown>);
    expect(container.querySelector("iframe")).toBeNull();
  });

  it("strips inline event handlers on HTML tags", () => {
    const body = "<img src='x' onerror='alert(1)' alt='bad'/>";
    const { container } = render(<Markdown>{body}</Markdown>);
    // skipHtml removes the raw HTML block entirely — img should not render.
    expect(container.querySelector("img")).toBeNull();
  });

  it("still renders GFM features (tables, task lists)", () => {
    const md = ["| a | b |", "|---|---|", "| 1 | 2 |"].join("\n");
    const { container } = render(<Markdown>{md}</Markdown>);
    expect(container.querySelector("table")).not.toBeNull();
    expect(container.querySelectorAll("td")).toHaveLength(2);
  });
});
