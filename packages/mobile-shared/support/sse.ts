// Minimal Server-Sent Events client for React Native. RN has no native
// EventSource, and fetch streaming is unreliable across engines, so this
// reads an XMLHttpRequest's incrementally-growing responseText (the same
// approach react-native-sse uses) — no native dependency. The wire parser is
// pure and unit-tested; openSSE is the thin RN-bound transport.
//
// Used as the support chat's middle fallback: WebSocket → SSE → polling.

/** Pure SSE frame parser. Feed raw text deltas; emits each event's `data`. */
export function createSSEParser(onData: (data: string) => void) {
  let buffer = "";
  return {
    push(chunk: string): void {
      // Normalise CRLF so framing is consistent across servers/proxies.
      buffer += chunk.replace(/\r\n/g, "\n").replace(/\r/g, "\n");
      let sep: number;
      while ((sep = buffer.indexOf("\n\n")) !== -1) {
        const rawEvent = buffer.slice(0, sep);
        buffer = buffer.slice(sep + 2);
        const dataLines: string[] = [];
        for (const line of rawEvent.split("\n")) {
          if (line.startsWith("data:")) {
            // Strip "data:" and an optional single leading space.
            dataLines.push(line.slice(5).replace(/^ /, ""));
          }
          // Lines starting with ":" are comments (heartbeats) — ignored.
        }
        if (dataLines.length > 0) onData(dataLines.join("\n"));
      }
    },
  };
}

export interface SSEHandle {
  close(): void;
}

export interface SSEHandlers {
  onOpen?: () => void;
  onMessage: (data: string) => void;
  onError?: () => void;
}

/**
 * Opens an SSE stream over XMLHttpRequest. Returns a handle whose close()
 * aborts the request. onError fires on transport failure or stream end so
 * the caller can reconnect.
 */
export function openSSE(url: string, h: SSEHandlers): SSEHandle {
  const xhr = new XMLHttpRequest();
  const parser = createSSEParser(h.onMessage);
  let lastIndex = 0;
  let errored = false;

  const fail = () => {
    if (errored) return;
    errored = true;
    h.onError?.();
  };

  xhr.open("GET", url, true);
  xhr.setRequestHeader("Accept", "text/event-stream");
  xhr.onreadystatechange = () => {
    // HEADERS_RECEIVED — confirm a 200 stream before declaring open.
    if (xhr.readyState === 2) {
      if (xhr.status === 200) {
        h.onOpen?.();
      } else {
        fail();
      }
    }
    // LOADING / DONE — drain whatever new text has arrived.
    if (xhr.readyState === 3 || xhr.readyState === 4) {
      const text = xhr.responseText ?? "";
      if (text.length > lastIndex) {
        const delta = text.slice(lastIndex);
        lastIndex = text.length;
        parser.push(delta);
      }
    }
    if (xhr.readyState === 4) {
      // Stream ended (server closed or network dropped) — always a
      // disconnect, so signal the caller to reconnect.
      fail();
    }
  };
  xhr.onerror = fail;
  xhr.ontimeout = fail;
  try {
    xhr.send();
  } catch {
    fail();
  }

  return {
    close() {
      errored = true; // suppress onError from the abort
      try {
        xhr.abort();
      } catch {
        /* noop */
      }
    },
  };
}
