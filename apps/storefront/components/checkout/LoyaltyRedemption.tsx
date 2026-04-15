"use client";

import { useState } from "react";

interface LoyaltyRedemptionProps {
  pointsBalance: number;
  pointsValue: string;
  pointsCurrency: string;
  currencyCode: string;
  minRedeemPoints: number;
  onToggle: (redeemPoints: number | null) => void;
}

function formatMoney(amount: number, currencyCode: string): string {
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency: currencyCode,
    }).format(amount);
  } catch {
    return `${currencyCode} ${amount.toFixed(2)}`;
  }
}

/**
 * Points redemption toggle shown inline under subtotal in the checkout
 * order totals section (NOT inside an accordion, per spec section 7.1).
 * Visible only when customer is enrolled and has redeemable points.
 */
export function LoyaltyRedemption({
  pointsBalance,
  pointsValue,
  pointsCurrency,
  currencyCode,
  minRedeemPoints,
  onToggle,
}: LoyaltyRedemptionProps) {
  const [isRedeeming, setIsRedeeming] = useState(false);
  const canRedeem = pointsBalance >= minRedeemPoints;

  const monetaryValue = pointsBalance * parseFloat(pointsValue);

  if (!canRedeem) {
    return (
      <div className="text-xs text-ink-500">
        You have {pointsBalance.toLocaleString()} {pointsCurrency} (min{" "}
        {minRedeemPoints} to redeem)
      </div>
    );
  }

  return (
    <div className="flex items-center justify-between py-2">
      <div>
        <p className="text-sm text-[color:var(--storefront-text,var(--ink-900))]">
          Use {pointsBalance.toLocaleString()} {pointsCurrency}
        </p>
        <p className="text-xs text-ink-500">
          Worth {formatMoney(monetaryValue, currencyCode)}
        </p>
      </div>
      <label className="relative inline-flex cursor-pointer items-center">
        <input
          type="checkbox"
          checked={isRedeeming}
          onChange={(e) => {
            setIsRedeeming(e.target.checked);
            onToggle(e.target.checked ? pointsBalance : null);
          }}
          className="peer sr-only"
          aria-label={`Redeem ${pointsBalance} ${pointsCurrency}`}
        />
        <div className="h-6 w-11 rounded-md bg-[color:var(--storefront-accent,var(--ink-900))]/10 after:absolute after:left-[2px] after:top-[2px] after:h-5 after:w-5 after:rounded-md after:bg-white after:transition-all peer-checked:bg-[color:var(--storefront-accent,var(--moss-700))] peer-checked:after:translate-x-full" />
      </label>
    </div>
  );
}
