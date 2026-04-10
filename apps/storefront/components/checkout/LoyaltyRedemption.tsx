"use client";

import { useState } from "react";

interface LoyaltyRedemptionProps {
  pointsBalance: number;
  pointsValue: string;
  pointsCurrency: string;
  minRedeemPoints: number;
  onToggle: (redeemPoints: number | null) => void;
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
  minRedeemPoints,
  onToggle,
}: LoyaltyRedemptionProps) {
  const [isRedeeming, setIsRedeeming] = useState(false);
  const canRedeem = pointsBalance >= minRedeemPoints;

  const monetaryValue = (pointsBalance * parseFloat(pointsValue)).toFixed(2);

  if (!canRedeem) {
    return (
      <div className="text-xs text-[color:var(--ink-900)]/40">
        You have {pointsBalance.toLocaleString()} {pointsCurrency} (min{" "}
        {minRedeemPoints} to redeem)
      </div>
    );
  }

  return (
    <div className="flex items-center justify-between py-2">
      <div>
        <p className="text-sm text-[color:var(--ink-900)]">
          Use {pointsBalance.toLocaleString()} {pointsCurrency}
        </p>
        <p className="text-xs text-[color:var(--ink-900)]/50">
          Worth ${monetaryValue}
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
        />
        <div className="h-6 w-11 rounded-full bg-[color:var(--ink-900)]/10 after:absolute after:left-[2px] after:top-[2px] after:h-5 after:w-5 after:rounded-full after:bg-white after:transition-all peer-checked:bg-[color:var(--moss-700)] peer-checked:after:translate-x-full" />
      </label>
    </div>
  );
}
