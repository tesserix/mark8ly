// @vitest-environment jsdom
//
// Covers the one property the mount-once resume effect promises, and the
// one mark8ly#1000 had to preserve by hand.
//
// That effect opens a socket and returns a teardown, so it must run EXACTLY
// once. Its dependency list is `[]`, which the rule would normally reject —
// `client`, `adoptConversation`, `teardownSocket` and `autoResume` are all
// read inside it. They are reached through a ref instead, so the empty list
// is honest rather than suppressed.
//
// A ref is easy to get subtly wrong: read it at the wrong time and you
// either re-resume on every render or capture a stale client forever. This
// asserts neither happens. Before this file nothing in the repo imported
// useSupportChat at all.

import { renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { useSupportChat } from "../useSupportChat";
import type { SupportClient } from "../client";

function stubClient(overrides: Partial<SupportClient> = {}): SupportClient {
  return {
    resume: vi.fn(async () => null),
    listMessages: vi.fn(async () => []),
    postMessage: vi.fn(async () => undefined),
    createConversation: vi.fn(async () => undefined),
    close: vi.fn(async () => undefined),
    getWsTicket: vi.fn(async () => null),
    buildWsUrl: vi.fn(() => ""),
    buildSseUrl: vi.fn(() => ""),
    ...overrides,
  } as unknown as SupportClient;
}

describe("useSupportChat — mount-once resume", () => {
  it("resumes exactly once, and not again on re-render", async () => {
    const client = stubClient();
    const { rerender } = renderHook(({ c }) => useSupportChat({ client: c }), {
      initialProps: { c: client },
    });

    await waitFor(() => expect(client.resume).toHaveBeenCalledTimes(1));

    rerender({ c: client });
    rerender({ c: client });

    expect(client.resume).toHaveBeenCalledTimes(1);
  });

  it("does not re-resume when the client identity changes", async () => {
    // The ref is what makes this true. Naming `client` as a dependency
    // would tear the socket down and resume again on a new client object,
    // which is a visible change in a live chat.
    const first = stubClient();
    const { rerender } = renderHook(({ c }) => useSupportChat({ client: c }), {
      initialProps: { c: first },
    });

    await waitFor(() => expect(first.resume).toHaveBeenCalledTimes(1));

    const second = stubClient();
    rerender({ c: second });

    expect(second.resume).not.toHaveBeenCalled();
    expect(first.resume).toHaveBeenCalledTimes(1);
  });

  it("does not resume at all when autoResume is false", async () => {
    const client = stubClient();
    const { result } = renderHook(() =>
      useSupportChat({ client, autoResume: false }),
    );

    await waitFor(() => expect(result.current.status).toBe("idle"));
    expect(client.resume).not.toHaveBeenCalled();
  });

  it("unmounts cleanly after a resume", async () => {
    const client = stubClient();
    const { unmount } = renderHook(() => useSupportChat({ client }));

    await waitFor(() => expect(client.resume).toHaveBeenCalledTimes(1));
    expect(() => unmount()).not.toThrow();
  });
});
