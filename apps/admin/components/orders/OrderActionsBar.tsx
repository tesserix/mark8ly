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
// server action and either toasts success and closes the panel, or
// surfaces the typed error inline next to the form for correction.

import { useCallback, useState, useTransition } from "react";
import type { ComponentType, SVGProps } from "react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@tesserix/web";
import { Check, CreditCard, PackageCheck, XCircle } from "lucide-react";

import { useToast } from "@/components/feedback/Toaster";
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

type Panel = "none" | "confirm" | "cancel" | "refund" | "fulfill";

export function OrderActionsBar({ order }: OrderActionsBarProps) {
  const { toast } = useToast();
  const [panel, setPanel] = useState<Panel>("none");
  const [pending, startTransition] = useTransition();
  const [error, setError] = useState<string | null>(null);

  const canConfirm = order.status === "pending";
  const canFulfill = order.status === "confirmed";
  const canCancel = order.status === "pending" || order.status === "confirmed";
  const canRefund =
    order.payment_status === "paid" ||
    order.payment_status === "partially_refunded";

  // OrderStatus is deliberately orthogonal to PaymentStatus (see
  // internal/order/status.go — "'refunded' is deliberately absent here and
  // lives on PaymentStatus only", spec §7 / §2 decision 4). So a fully
  // refunded order legitimately still sits at status="confirmed" and CAN
  // be fulfilled — a goodwill refund where the goods still ship.
  //
  // Legal is not the same as likely, though. Shipping goods you've already
  // refunded in full is the rare case, so it stops being the primary
  // action and gains a confirm step; cancelling — which is what actually
  // closes the order out and restocks — becomes primary instead. A
  // PARTIAL refund changes nothing: the rest of the order still ships.
  const isFullyRefunded = order.payment_status === "refunded";

  const close = useCallback(() => {
    setPanel("none");
    setError(null);
  }, []);

  const onSuccess = useCallback(
    (title: string, description?: string) => {
      close();
      toast.success(title, description);
    },
    [close, toast],
  );

  const runFulfill = () => {
    setError(null);
    startTransition(async () => {
      const r = await fulfillOrderAction(order.store_id, order.id);
      if (!r.ok) {
        const msg = r.error?.message ?? "Action failed";
        setError(msg);
        toast.error("Couldn't mark fulfilled", msg);
      } else {
        onSuccess("Order marked fulfilled");
      }
    });
  };

  return (
    <section aria-label="Order actions" className="flex flex-col gap-4">
      {/* State the refund outright. Otherwise it can only be inferred from
          the absence of "Issue refund", and the remaining fulfil/cancel
          buttons read as though the order were still live. */}
      {isFullyRefunded && (
        <p className="text-sm text-foreground-secondary">
          <span className="font-medium text-foreground">Fully refunded.</span>{" "}
          The customer has been repaid in full — cancel to close this order
          out and return the items to stock.
        </p>
      )}

      <div className="flex flex-wrap items-center gap-2">
        {canConfirm && (
          <ActionButton
            label="Confirm order"
            icon={Check}
            primary
            active={panel === "confirm"}
            disabled={pending}
            onClick={() => setPanel(panel === "confirm" ? "none" : "confirm")}
          />
        )}
        {canFulfill && (
          <ActionButton
            label={pending ? "Marking…" : "Mark fulfilled"}
            icon={PackageCheck}
            primary={!isFullyRefunded}
            active={panel === "fulfill"}
            disabled={pending}
            onClick={
              isFullyRefunded
                ? () => setPanel(panel === "fulfill" ? "none" : "fulfill")
                : runFulfill
            }
          />
        )}
        {canCancel && (
          <ActionButton
            label="Cancel order"
            icon={XCircle}
            primary={isFullyRefunded}
            tone={isFullyRefunded ? "default" : "danger"}
            active={panel === "cancel"}
            disabled={pending}
            onClick={() => setPanel(panel === "cancel" ? "none" : "cancel")}
          />
        )}
        {canRefund && (
          <ActionButton
            label="Issue refund"
            icon={CreditCard}
            active={panel === "refund"}
            disabled={pending}
            onClick={() => setPanel(panel === "refund" ? "none" : "refund")}
          />
        )}
        {!canConfirm && !canFulfill && !canCancel && !canRefund && (
          <span className="text-sm text-foreground-tertiary">
            No further actions available for this order.
          </span>
        )}
      </div>

      {error && (
        <p
          role="alert"
          className="text-sm text-[color:var(--danger)]"
        >
          {error}
        </p>
      )}

      {panel === "fulfill" && (
        <PanelShell title="Fulfil a refunded order?">
          <p className="text-sm text-foreground-secondary">
            This order has been <strong>fully refunded</strong> — the customer
            has their money back. Marking it fulfilled says you are shipping
            the goods anyway, and cannot be undone.
          </p>
          <p className="text-sm text-foreground-secondary">
            If you meant to close this order out, cancel it instead — that
            also returns the items to stock.
          </p>
          <div className="flex items-center gap-2">
            <button
              type="button"
              disabled={pending}
              onClick={runFulfill}
              className="inline-flex items-center gap-2 rounded-md bg-[color:var(--ink-900)] px-4 py-2 text-sm font-medium text-[color:var(--primary-foreground)] transition-colors hover:bg-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-40 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
            >
              {pending ? "Marking…" : "Yes, mark fulfilled"}
            </button>
            <button
              type="button"
              onClick={close}
              className="text-sm text-foreground-secondary underline underline-offset-4 hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)]"
            >
              Cancel
            </button>
          </div>
        </PanelShell>
      )}
      {panel === "confirm" && (
        <ConfirmForm
          orderId={order.id}
          storeId={order.store_id}
          onDone={(msg) => (msg ? onSuccess("Order confirmed", msg) : close())}
        />
      )}
      {panel === "cancel" && (
        <CancelForm
          orderId={order.id}
          storeId={order.store_id}
          customerEmail={order.customer_email}
          onDone={(msg) => (msg ? onSuccess("Order cancelled", msg) : close())}
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
          onDone={(msg) => (msg ? onSuccess("Refund issued", msg) : close())}
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
  icon?: ComponentType<SVGProps<SVGSVGElement>>;
  primary?: boolean;
  tone?: "default" | "danger";
  active?: boolean;
  disabled?: boolean;
  onClick: () => void;
}

function ActionButton({
  label,
  icon: Icon,
  primary,
  tone = "default",
  active,
  disabled,
  onClick,
}: ActionButtonProps) {
  const base =
    "inline-flex h-8 items-center gap-1.5 rounded-md px-2.5 text-[13px] font-medium transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50";
  let cls = "";
  if (primary) {
    cls = active
      ? "bg-[color:var(--moss-700)] text-[color:var(--primary-foreground)]"
      : "bg-[color:var(--ink-900)] text-[color:var(--primary-foreground)] hover:bg-[color:var(--moss-700)]";
  } else if (tone === "danger") {
    cls = active
      ? "border border-[color:var(--danger)] text-[color:var(--danger)]"
      : "border border-[color:var(--ink-900)]/20 text-foreground-secondary hover:border-[color:var(--danger)] hover:text-[color:var(--danger)]";
  } else {
    cls = active
      ? "border border-[color:var(--moss-700)] text-[color:var(--moss-700)]"
      : "border border-[color:var(--ink-900)]/20 text-foreground hover:border-[color:var(--moss-700)] hover:text-[color:var(--moss-700)]";
  }
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className={`${base} ${cls}`}
    >
      {Icon && <Icon className="h-3.5 w-3.5" aria-hidden="true" />}
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
  // Hairline-top inline continuation. The form reads as an elaboration
  // of the action button above it, not as a callout — the destructive
  // weight of cancel/refund comes from button color and two-step copy,
  // not from a border treatment around the workspace.
  return (
    <div className="flex flex-col gap-4 border-t border-border-subtle pt-4">
      <h3 className="text-xs font-semibold uppercase tracking-[0.12em] text-foreground-tertiary">
        {title}
      </h3>
      {children}
    </div>
  );
}

function FieldLabel({ children }: { children: React.ReactNode }) {
  return (
    <span className="text-xs uppercase tracking-wider text-foreground-tertiary">
      {children}
    </span>
  );
}

function FormError({ error }: { error: OrderActionResult["error"] | undefined }) {
  if (!error) return null;
  return (
    <p role="alert" className="text-sm text-[color:var(--danger)]">
      {error.message || "Something went wrong. Please try again."}
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
      const desc = paymentStatus
        ? `Payment marked as ${paymentStatus}.`
        : "Order moved to confirmed.";
      onDone(desc);
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
      onDone(`${customerEmail} has been notified.`);
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
          <SubmitButton pending={pending} label="Cancel order" onClick={submit} />
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
  // Stable per-mount id: same value across re-renders and submit retries
  // within this open form, but a new id if the form is closed and reopened
  // (a new refund attempt). This is the idempotency semantics we want.
  const [refundRequestId] = useState(() => crypto.randomUUID());

  const remaining = (
    Number.parseFloat(grandTotal) - Number.parseFloat(refundedAmount)
  ).toFixed(2);

  // "Fully refunded" has exactly one correct amount — the refundable
  // remainder — so fill it in rather than make the merchant retype a figure
  // already shown directly above the field. Switching back to partial clears
  // it, so a full amount can't be submitted as a partial refund by accident.
  // A manual edit afterwards still wins; this only seeds the value.
  const selectPaymentStatus = (next: PaymentStatus) => {
    setPaymentStatus(next);
    setAmount(next === "refunded" ? remaining : "");
  };

  const submit = () => {
    setError(undefined);
    startTransition(async () => {
      const r = await refundOrderAction(storeId, orderId, {
        amount,
        paymentStatus,
        reason,
        refundRequestId,
      });
      if (!r.ok) {
        setError(r.error);
        setStep("form");
        return;
      }
      onDone(`${currencyCode} ${amount} refunded to ${customerEmail}.`);
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
          <p className="text-[color:var(--danger)]">
            This cannot be undone.
          </p>
          {reason && <p className="opacity-70">Note: {reason}</p>}
        </div>
        <FormError error={error} />
        <div className="flex items-center gap-3">
          <SubmitButton pending={pending} label="Confirm refund" onClick={submit} />
          <DismissButton onClick={() => setStep("form")} disabled={pending} label="Go back" />
        </div>
      </PanelShell>
    );
  }

  return (
    <PanelShell title="Issue a refund">
      <p className="text-sm text-foreground-secondary">
        Refundable: {currencyCode} {remaining}
      </p>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          setStep("review");
        }}
        className="flex flex-col gap-4"
      >
        {/* Refund type leads: it's the decision the merchant actually makes,
            and picking "Fully refunded" fills in the amount below — which
            only reads correctly when the cause sits above the effect. */}
        <div className="flex flex-col gap-2">
          <FieldLabel>Mark order as</FieldLabel>
          <Select
            value={paymentStatus}
            onValueChange={(value) => selectPaymentStatus(value as PaymentStatus)}
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

function SubmitButton({
  pending,
  label,
  onClick,
}: {
  pending: boolean;
  label: string;
  // Optional click handler — when the button isn't inside a <form>
  // (e.g. the cancel/refund "review" panels) the action must wire its
  // own onClick or nothing happens.
  onClick?: () => void;
}) {
  return (
    <button
      type={onClick ? "button" : "submit"}
      disabled={pending}
      onClick={onClick}
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
      className="text-sm text-foreground-secondary hover:opacity-100 hover:text-[color:var(--moss-700)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-50"
    >
      {label}
    </button>
  );
}
