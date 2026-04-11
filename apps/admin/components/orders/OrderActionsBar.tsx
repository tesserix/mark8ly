"use client";

// components/orders/OrderActionsBar.tsx
//
// Action surface for the order detail page. Four actions, each gated on
// the order's current status:
//
//   • Confirm  — visible when status === "pending"
//   • Fulfill  — visible when status === "confirmed"
//   • Cancel   — visible when status === "pending" || "confirmed"
//   • Refund   — visible when payment_status ∈ {"paid","partially_refunded"}
//
// Inline expanding form panels (no modal dialogs). Pressing an action
// button reveals its form below the bar; submitting the form runs the
// server action and either closes the panel + lets the page revalidate,
// or surfaces the typed error inline.

import { useCallback, useEffect, useState, useTransition } from "react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@tesserix/web";

import type { AdminOrder, PaymentStatus } from "@/lib/api/marketplace-api";

import {
  cancelOrderAction,
  confirmOrderAction,
  fulfillOrderAction,
  refundOrderAction,
  type OrderActionResult,
} from "@/app/(admin)/orders/[id]/actions";

interface OrderActionsBarProps {
  order: AdminOrder;
}

type Panel = "none" | "confirm" | "cancel" | "refund";

export function OrderActionsBar({ order }: OrderActionsBarProps) {
  const [panel, setPanel] = useState<Panel>("none");
  const [pending, startTransition] = useTransition();
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  const canConfirm = order.status === "pending";
  const canFulfill = order.status === "confirmed";
  const canCancel = order.status === "pending" || order.status === "confirmed";
  const canRefund =
    order.payment_status === "paid" ||
    order.payment_status === "partially_refunded";

  // Auto-clear success strip after 4 seconds.
  useEffect(() => {
    if (!success) return;
    const t = setTimeout(() => setSuccess(null), 4000);
    return () => clearTimeout(t);
  }, [success]);

  const close = useCallback(() => {
    setPanel("none");
    setError(null);
  }, []);

  const onSuccess = useCallback((msg: string) => {
    close();
    setSuccess(msg);
  }, [close]);

  const runFulfill = () => {
    setError(null);
    startTransition(async () => {
      const r = await fulfillOrderAction(order.store_id, order.id);
      if (!r.ok) {
        setError(r.error?.message ?? "Action failed");
      } else {
        onSuccess("Order marked as fulfilled.");
      }
    });
  };

  return (
    <section
      aria-labelledby="order-actions-heading"
      className="flex flex-col gap-4"
    >
      <h2
        id="order-actions-heading"
        className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-xl text-[color:var(--ink-900)]"
      >
        Actions
      </h2>

      {/* Success feedback strip — editorial style: serif italic, hairline rules. */}
      {success && (
        <div
          role="status"
          className="border-y border-[color:var(--moss-700)] border-opacity-40 py-3"
        >
          <p className="font-[family-name:var(--font-serif,'Source_Serif_4',serif)] text-sm italic text-[color:var(--moss-700)]">
            {success}
          </p>
        </div>
      )}

      <div className="flex flex-wrap items-center gap-3">
        {canConfirm && (
          <ActionButton
            label="Confirm order"
            primary
            active={panel === "confirm"}
            disabled={pending}
            onClick={() => setPanel(panel === "confirm" ? "none" : "confirm")}
          />
        )}
        {canFulfill && (
          <ActionButton
            label={pending ? "Marking…" : "Mark fulfilled"}
            primary
            disabled={pending}
            onClick={runFulfill}
          />
        )}
        {canCancel && (
          <ActionButton
            label="Cancel order"
            active={panel === "cancel"}
            disabled={pending}
            onClick={() => setPanel(panel === "cancel" ? "none" : "cancel")}
          />
        )}
        {canRefund && (
          <ActionButton
            label="Issue refund"
            active={panel === "refund"}
            disabled={pending}
            onClick={() => setPanel(panel === "refund" ? "none" : "refund")}
          />
        )}
        {!canConfirm && !canFulfill && !canCancel && !canRefund && (
          <span className="text-sm text-[color:var(--ink-900)] opacity-60">
            No further actions available for this order.
          </span>
        )}
      </div>

      {error && (
        <p
          role="alert"
          className="text-sm text-[color:var(--danger,#5a1010)]"
        >
          {error}
        </p>
      )}

      {panel === "confirm" && (
        <ConfirmForm
          orderId={order.id}
          storeId={order.store_id}
          onDone={(msg) => onSuccess(msg ?? "Order confirmed.")}
        />
      )}
      {panel === "cancel" && (
        <CancelForm
          orderId={order.id}
          storeId={order.store_id}
          customerEmail={order.customer_email}
          onDone={(msg) => onSuccess(msg ?? "Order cancelled.")}
        />
      )}
      {panel === "refund" && (
        <RefundForm
          orderId={order.id}
          storeId={order.store_id}
          grandTotal={order.grand_total}
          refundedAmount={order.refunded_amount}
          currencyCode={order.currency_code}
          customerEmail={order.customer_email}
          onDone={(msg) => onSuccess(msg ?? "Refund issued.")}
        />
      )}
    </section>
  );
}

// ─────────────────────────────────────────────────────────────────────────
// Buttons
// ─────────────────────────────────────────────────────────────────────────

interface ActionButtonProps {
  label: string;
  primary?: boolean;
  active?: boolean;
  disabled?: boolean;
  onClick: () => void;
}

function ActionButton({
  label,
  primary,
  active,
  disabled,
  onClick,
}: ActionButtonProps) {
  const base =
    "inline-flex items-center gap-2 rounded-md px-4 py-2 text-sm transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50";
  let cls = "";
  if (primary) {
    cls = active
      ? "bg-[color:var(--moss-700)] text-[color:var(--paper-200)]"
      : "bg-[color:var(--ink-900)] text-[color:var(--paper-200)] hover:bg-[color:var(--moss-700)]";
  } else {
    cls = active
      ? "border border-[color:var(--moss-700)] text-[color:var(--moss-700)]"
      : "border border-[color:var(--ink-900)] border-opacity-40 text-[color:var(--ink-900)] hover:border-[color:var(--moss-700)] hover:text-[color:var(--moss-700)]";
  }
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className={`${base} ${cls}`}
    >
      {label}
    </button>
  );
}

// ─────────────────────────────────────────────────────────────────────────
// Inline forms
// ─────────────────────────────────────────────────────────────────────────

function PanelShell({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-4 border-l-2 border-[color:var(--moss-700)] bg-[color:var(--ink-900)] bg-opacity-[0.02] px-5 py-4">
      <h3 className="text-base text-[color:var(--ink-900)]">{title}</h3>
      {children}
    </div>
  );
}

function FieldLabel({ children }: { children: React.ReactNode }) {
  return (
    <span className="text-xs uppercase tracking-wider text-[color:var(--ink-900)] opacity-60">
      {children}
    </span>
  );
}

function FormError({ error }: { error: OrderActionResult["error"] | undefined }) {
  if (!error) return null;
  return (
    <p role="alert" className="text-sm text-[color:var(--danger,#5a1010)]">
      {error.message}
    </p>
  );
}

interface ConfirmFormProps {
  storeId: string;
  orderId: string;
  onDone: (msg?: string) => void;
}

function ConfirmForm({ storeId, orderId, onDone }: ConfirmFormProps) {
  const [pending, startTransition] = useTransition();
  const [error, setError] = useState<OrderActionResult["error"] | undefined>();
  const [paymentStatus, setPaymentStatus] = useState<PaymentStatus | "">("");
  const [reason, setReason] = useState("");

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    setError(undefined);
    startTransition(async () => {
      const r = await confirmOrderAction(storeId, orderId, {
        paymentStatus: paymentStatus === "" ? undefined : paymentStatus,
        reason,
      });
      if (!r.ok) {
        setError(r.error);
        return;
      }
      const ps = paymentStatus ? ` Payment marked as ${paymentStatus}.` : "";
      onDone(`Order confirmed.${ps}`);
    });
  };

  return (
    <PanelShell title="Confirm this order">
      <form onSubmit={submit} className="flex flex-col gap-4">
        <div className="flex flex-col gap-2">
          <FieldLabel>Payment status (optional)</FieldLabel>
          <Select
            value={paymentStatus || "__noop__"}
            onValueChange={(value) =>
              setPaymentStatus(
                value === "__noop__" ? "" : (value as PaymentStatus),
              )
            }
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="__noop__">Don&apos;t change</SelectItem>
              <SelectItem value="authorized">Authorized</SelectItem>
              <SelectItem value="paid">Paid</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <label className="flex flex-col gap-2">
          <FieldLabel>Note (optional)</FieldLabel>
          <input
            type="text"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder="e.g. stripe-auth"
            className={inputCls}
          />
        </label>
        <FormError error={error} />
        <div className="flex items-center gap-3">
          <SubmitButton pending={pending} label="Confirm" />
          <DismissButton onClick={() => onDone()} disabled={pending} />
        </div>
      </form>
    </PanelShell>
  );
}

// ── Cancel (two-step: form → review → confirm) ──────────────────────────

interface CancelFormProps {
  storeId: string;
  orderId: string;
  customerEmail: string;
  onDone: (msg?: string) => void;
}

function CancelForm({ storeId, orderId, customerEmail, onDone }: CancelFormProps) {
  const [pending, startTransition] = useTransition();
  const [error, setError] = useState<OrderActionResult["error"] | undefined>();
  const [reason, setReason] = useState("");
  const [step, setStep] = useState<"form" | "review">("form");

  const submit = () => {
    setError(undefined);
    startTransition(async () => {
      const r = await cancelOrderAction(storeId, orderId, { reason });
      if (!r.ok) {
        setError(r.error);
        setStep("form");
        return;
      }
      onDone("Order cancelled.");
    });
  };

  if (step === "review") {
    return (
      <PanelShell title="Review cancellation">
        <div className="flex flex-col gap-3 text-sm text-[color:var(--ink-900)]">
          <p>You are about to <strong>cancel</strong> this order. This cannot be undone.</p>
          <p className="opacity-70">Customer ({customerEmail}) will be notified.</p>
          {reason && <p className="opacity-70">Reason: {reason}</p>}
        </div>
        <FormError error={error} />
        <div className="flex items-center gap-3">
          <SubmitButton pending={pending} label="Cancel order" />
          <DismissButton onClick={() => setStep("form")} disabled={pending} label="Go back" />
        </div>
      </PanelShell>
    );
  }

  return (
    <PanelShell title="Cancel this order">
      <form
        onSubmit={(e) => {
          e.preventDefault();
          setStep("review");
        }}
        className="flex flex-col gap-4"
      >
        <label className="flex flex-col gap-2">
          <FieldLabel>Reason (required)</FieldLabel>
          <input
            type="text"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            required
            placeholder="e.g. customer requested cancellation"
            className={inputCls}
          />
        </label>
        <div className="flex items-center gap-3">
          <SubmitButton pending={false} label="Review" />
          <DismissButton onClick={() => onDone()} />
        </div>
      </form>
    </PanelShell>
  );
}

// ── Refund (two-step: form → review → confirm) ─────────────────────────

interface RefundFormProps {
  storeId: string;
  orderId: string;
  grandTotal: string;
  refundedAmount: string;
  currencyCode: string;
  customerEmail: string;
  onDone: (msg?: string) => void;
}

function RefundForm({
  storeId,
  orderId,
  grandTotal,
  refundedAmount,
  currencyCode,
  customerEmail,
  onDone,
}: RefundFormProps) {
  const [pending, startTransition] = useTransition();
  const [error, setError] = useState<OrderActionResult["error"] | undefined>();
  const [amount, setAmount] = useState("");
  const [paymentStatus, setPaymentStatus] = useState<PaymentStatus>(
    "partially_refunded",
  );
  const [reason, setReason] = useState("");
  const [step, setStep] = useState<"form" | "review">("form");

  const remaining = (
    Number.parseFloat(grandTotal) - Number.parseFloat(refundedAmount)
  ).toFixed(2);

  const submit = () => {
    setError(undefined);
    startTransition(async () => {
      const r = await refundOrderAction(storeId, orderId, {
        amount,
        paymentStatus,
        reason,
      });
      if (!r.ok) {
        setError(r.error);
        setStep("form");
        return;
      }
      onDone(`Refund of ${currencyCode} ${amount} issued.`);
    });
  };

  if (step === "review") {
    const label = paymentStatus === "refunded" ? "fully refunded" : "partially refunded";
    return (
      <PanelShell title="Review refund">
        <div className="flex flex-col gap-3 text-sm text-[color:var(--ink-900)]">
          <p>
            You are about to refund{" "}
            <strong>{currencyCode} {amount}</strong> to {customerEmail}.
            The order will be marked <strong>{label}</strong>.
          </p>
          <p className="text-[color:var(--danger,#5a1010)]">
            This cannot be undone.
          </p>
          {reason && <p className="opacity-70">Note: {reason}</p>}
        </div>
        <FormError error={error} />
        <div className="flex items-center gap-3">
          <SubmitButton pending={pending} label="Confirm refund" />
          <DismissButton onClick={() => setStep("form")} disabled={pending} label="Go back" />
        </div>
      </PanelShell>
    );
  }

  return (
    <PanelShell title="Issue a refund">
      <p className="text-sm text-[color:var(--ink-900)] opacity-70">
        Refundable: {currencyCode} {remaining}
      </p>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          setStep("review");
        }}
        className="flex flex-col gap-4"
      >
        <label className="flex flex-col gap-2">
          <FieldLabel>Amount</FieldLabel>
          <input
            type="number"
            step="0.01"
            min="0.01"
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            required
            placeholder="0.00"
            className={inputCls}
          />
        </label>
        <div className="flex flex-col gap-2">
          <FieldLabel>Mark order as</FieldLabel>
          <Select
            value={paymentStatus}
            onValueChange={(value) =>
              setPaymentStatus(value as PaymentStatus)
            }
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="partially_refunded">
                Partially refunded
              </SelectItem>
              <SelectItem value="refunded">Fully refunded</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <label className="flex flex-col gap-2">
          <FieldLabel>Note (optional)</FieldLabel>
          <input
            type="text"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder="e.g. damaged in transit"
            className={inputCls}
          />
        </label>
        <div className="flex items-center gap-3">
          <SubmitButton pending={false} label="Review" />
          <DismissButton onClick={() => onDone()} />
        </div>
      </form>
    </PanelShell>
  );
}

// ─────────────────────────────────────────────────────────────────────────
// Shared form primitives
// ─────────────────────────────────────────────────────────────────────────

const inputCls =
  "rounded-md border border-[color:var(--ink-900)] border-opacity-30 bg-[color:var(--background-elevated,white)] px-3 py-2 text-sm text-[color:var(--ink-900)] focus-visible:border-[color:var(--moss-700)] focus-visible:outline-none";

function SubmitButton({ pending, label }: { pending: boolean; label: string }) {
  return (
    <button
      type="submit"
      disabled={pending}
      className="inline-flex items-center gap-2 rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm text-[color:var(--paper-200)] transition-colors hover:bg-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50"
    >
      {pending ? "Working…" : label}
    </button>
  );
}

function DismissButton({
  onClick,
  disabled,
  label = "Cancel",
}: {
  onClick: () => void;
  disabled?: boolean;
  label?: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className="text-sm text-[color:var(--ink-900)] opacity-70 hover:opacity-100 hover:text-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50"
    >
      {label}
    </button>
  );
}
