"use client";

import { useState, useTransition } from "react";

import { requestNewReceiptLink } from "@/app/r/[token]/actions";

export function NewLinkButton({ token }: { token: string }) {
  const [done, setDone] = useState<boolean | null>(null);
  const [pending, start] = useTransition();
  if (done) {
    return <p className="text-sm">If there&rsquo;s an email on this order, we&rsquo;ve sent a new link to it.</p>;
  }
  return (
    <div className="flex flex-col gap-2 print:hidden">
      <p className="text-sm opacity-80">Payment and contact details are hidden on older links.</p>
      <button
        type="button"
        disabled={pending}
        onClick={() => start(async () => setDone(await requestNewReceiptLink(token)))}
        className="min-h-11 self-start rounded-md border border-current px-4 text-sm"
      >
        {pending ? "Sending…" : "Send a new link"}
      </button>
      {done === false ? <p className="text-sm">Couldn&rsquo;t send a new link. Try again later.</p> : null}
    </div>
  );
}
