"use client";

import * as React from "react";

import { receiptAction } from "@/app/(admin)/sell/actions";

// Email or print the receipt for an in-store sale (#1106, #1107). Print
// opens the receipt page; the tablet's own print (AirPrint) does the rest.
export function ReceiptActions({ storeId, orderId }: { storeId: string; orderId: string }) {
  const [email, setEmail] = React.useState("");
  const [pending, setPending] = React.useState(false);
  const [sentTo, setSentTo] = React.useState<string | null>(null);
  const [error, setError] = React.useState<string | null>(null);
  const looksValid = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim());

  async function run(send: boolean) {
    setPending(true);
    setError(null);
    setSentTo(null);
    const addr = email.trim();
    const res = await receiptAction(storeId, orderId, send ? { send: true, email: addr } : { send: false });
    setPending(false);
    if (!res.ok) return setError(res.message);
    if (send) setSentTo(addr);
    else window.open(res.url, "_blank", "noopener");
  }

  return (
    <div className="flex w-full flex-col gap-3">
      <label className="flex flex-col gap-1 text-sm">
        Customer&rsquo;s email
        <input
          type="email"
          inputMode="email"
          autoComplete="off"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          className="h-11 rounded-md border border-border bg-surface px-3 text-base"
        />
      </label>
      <div className="flex flex-wrap gap-2">
        <button
          type="button"
          disabled={pending || !looksValid}
          onClick={() => run(true)}
          className="min-h-11 rounded-md border border-border px-4 text-sm disabled:opacity-50"
        >
          Email receipt
        </button>
        <button
          type="button"
          disabled={pending}
          onClick={() => run(false)}
          className="min-h-11 rounded-md border border-border px-4 text-sm disabled:opacity-50"
        >
          Print receipt
        </button>
      </div>
      {sentTo ? <p className="text-sm">Receipt sent to {sentTo}.</p> : null}
      {error ? (
        <p role="alert" className="text-sm text-danger">
          {error}
        </p>
      ) : null}
    </div>
  );
}
