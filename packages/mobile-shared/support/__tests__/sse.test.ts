import { describe, it, expect } from "vitest";
import { createSSEParser } from "../sse";

function collect(chunks: string[]): string[] {
  const out: string[] = [];
  const parser = createSSEParser((d) => out.push(d));
  for (const c of chunks) parser.push(c);
  return out;
}

describe("createSSEParser", () => {
  it("parses a single complete event", () => {
    expect(collect(['data: {"type":"x"}\n\n'])).toEqual(['{"type":"x"}']);
  });

  it("handles an event split across chunks", () => {
    expect(collect(['data: {"ty', 'pe":"x"}', "\n\n"])).toEqual(['{"type":"x"}']);
  });

  it("parses multiple events in one chunk", () => {
    expect(collect(["data: a\n\ndata: b\n\n"])).toEqual(["a", "b"]);
  });

  it("ignores comment heartbeats", () => {
    expect(collect([": ping\n\n", "data: real\n\n"])).toEqual(["real"]);
  });

  it("joins multi-line data fields", () => {
    expect(collect(["data: line1\ndata: line2\n\n"])).toEqual(["line1\nline2"]);
  });

  it("strips only one leading space after data:", () => {
    expect(collect(["data:  two-spaces\n\n"])).toEqual([" two-spaces"]);
  });

  it("normalises CRLF framing", () => {
    expect(collect(['data: {"a":1}\r\n\r\n'])).toEqual(['{"a":1}']);
  });

  it("buffers an incomplete event until its terminator arrives", () => {
    const out: string[] = [];
    const p = createSSEParser((d) => out.push(d));
    p.push("data: partial\n");
    expect(out).toEqual([]);
    p.push("\n");
    expect(out).toEqual(["partial"]);
  });
});
