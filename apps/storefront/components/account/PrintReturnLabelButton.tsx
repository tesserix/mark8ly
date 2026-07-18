"use client";

// Client-only print trigger for the return-label page. The page itself
// is a server component (fetches order + return data via cookies()),
// so this small island isolates the one piece of interactivity that
// needs the browser's window.print(). Previously this was a
// <form action="javascript:window.print()"> — fragile (some browsers
// block javascript: URIs outright) and a needless CSP risk for a
// same-page action that has no business submitting anywhere.

export function PrintReturnLabelButton() {
  function handlePrint() {
    if (typeof window === "undefined" || typeof window.print !== "function") {
      return;
    }
    window.print();
  }

  return (
    <div className="inline-flex items-center gap-3">
      <button
        type="button"
        onClick={handlePrint}
        className="rounded-md bg-black px-5 py-2.5 text-sm font-medium text-white transition-opacity hover:opacity-90"
      >
        Print this page
      </button>
      <span className="text-xs text-black/50">
        Quote {"{RMA}"} if you contact support.
      </span>
    </div>
  );
}
