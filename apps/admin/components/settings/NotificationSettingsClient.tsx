"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import {
  ShoppingCart,
  AlertTriangle,
  RotateCcw,
  DollarSign,
  Star,
} from "lucide-react";

import type { NotificationPreferences } from "@/lib/api/settings-tier2-api";
import { updateNotificationPreferencesAction } from "@/app/settings/actions";

interface NotificationSettingsClientProps {
  preferences: NotificationPreferences | null;
  editable: boolean;
}

interface NotificationToggle {
  key: keyof NotificationPreferences;
  label: string;
  description: string;
  icon: typeof ShoppingCart;
}

const NOTIFICATION_TYPES: NotificationToggle[] = [
  {
    key: "new_order",
    label: "New orders",
    description: "Get notified when a customer places a new order.",
    icon: ShoppingCart,
  },
  {
    key: "low_stock",
    label: "Low stock alerts",
    description: "Get notified when product inventory falls below threshold.",
    icon: AlertTriangle,
  },
  {
    key: "return_requested",
    label: "Return requests",
    description: "Get notified when a customer requests a return.",
    icon: RotateCcw,
  },
  {
    key: "payment_received",
    label: "Payments received",
    description: "Get notified when a payment is successfully processed.",
    icon: DollarSign,
  },
  {
    key: "review_submitted",
    label: "Product reviews",
    description: "Get notified when a customer submits a product review.",
    icon: Star,
  },
];

const DEFAULT_PREFS: NotificationPreferences = {
  new_order: true,
  low_stock: true,
  return_requested: true,
  payment_received: false,
  review_submitted: false,
};

export function NotificationSettingsClient({
  preferences,
  editable,
}: NotificationSettingsClientProps) {
  const prefs = preferences ?? DEFAULT_PREFS;
  const [localPrefs, setLocalPrefs] = useState<NotificationPreferences>(prefs);
  const [isPending, startTransition] = useTransition();
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);
  const router = useRouter();

  const hasChanges = NOTIFICATION_TYPES.some(
    (t) => localPrefs[t.key] !== prefs[t.key],
  );

  function handleToggle(key: keyof NotificationPreferences) {
    setSuccess(false);
    setLocalPrefs((prev) => ({ ...prev, [key]: !prev[key] }));
  }

  function handleSave() {
    setError(null);
    setSuccess(false);
    startTransition(async () => {
      const result = await updateNotificationPreferencesAction(localPrefs);
      if (!result.ok) {
        setError(result.message);
      } else {
        setSuccess(true);
        router.refresh();
      }
    });
  }

  return (
    <div className="space-y-8">
      <div className="space-y-1">
        {NOTIFICATION_TYPES.map((type) => {
          const Icon = type.icon;
          const enabled = localPrefs[type.key];
          return (
            <div
              key={type.key}
              className="flex items-center justify-between border-b border-border py-5 last:border-b-0"
            >
              <div className="flex items-start gap-4">
                <Icon
                  className="mt-0.5 h-5 w-5 text-foreground-secondary"
                  aria-hidden="true"
                />
                <div className="space-y-0.5">
                  <p className="text-sm font-medium text-foreground">{type.label}</p>
                  <p className="text-sm text-foreground-secondary">{type.description}</p>
                </div>
              </div>
              <button
                type="button"
                role="switch"
                aria-checked={enabled}
                aria-label={`${type.label}: ${enabled ? "enabled" : "disabled"}`}
                onClick={() => editable && handleToggle(type.key)}
                disabled={!editable || isPending}
                className={`relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[color:var(--moss-700)] focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 ${
                  enabled ? "bg-[color:var(--moss-700)]" : "bg-[color:var(--ink-900)]/20"
                }`}
              >
                <span
                  className={`pointer-events-none block h-5 w-5 rounded-full bg-white shadow-sm ring-0 transition-transform ${
                    enabled ? "translate-x-5" : "translate-x-0"
                  }`}
                />
              </button>
            </div>
          );
        })}
      </div>
      {error && <p className="text-sm text-[color:var(--signal)]">{error}</p>}
      {success && <p className="text-sm text-[color:var(--moss-700)]">Preferences saved.</p>}
      {editable && (
        <button
          type="button"
          onClick={handleSave}
          disabled={isPending || !hasChanges}
          className="h-10 rounded-[6px] bg-[color:var(--ink-900)] px-5 text-sm font-medium text-white transition-colors hover:bg-[color:var(--ink-900)]/90 disabled:opacity-50"
        >
          {isPending ? "Saving..." : "Save preferences"}
        </button>
      )}
    </div>
  );
}
