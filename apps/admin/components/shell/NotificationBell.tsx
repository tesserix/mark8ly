"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { formatDistanceToNow } from "date-fns";
import {
  Bell,
  ShoppingCart,
  AlertTriangle,
  RotateCcw,
  DollarSign,
  Star,
  CheckCheck,
  Settings,
} from "lucide-react";

interface NotificationItem {
  id: string;
  type: string;
  title: string;
  message: string | null;
  is_read: boolean;
  created_at: string;
}

interface NotificationBellProps {
  storeId: string;
  userId: string;
  tenantId: string;
}

const TYPE_ICONS: Record<string, typeof Bell> = {
  new_order: ShoppingCart,
  low_stock: AlertTriangle,
  return_requested: RotateCcw,
  payment_received: DollarSign,
  review_submitted: Star,
};

export function NotificationBell({
  storeId,
  userId,
  tenantId,
}: NotificationBellProps) {
  const [open, setOpen] = useState(false);
  const [unreadCount, setUnreadCount] = useState(0);
  const [notifications, setNotifications] = useState<NotificationItem[]>([]);
  const [loading, setLoading] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);

  // Poll unread count every 30 seconds
  const fetchUnreadCount = useCallback(async () => {
    try {
      const res = await fetch(
        `/api/notifications/unread-count?storeId=${storeId}`,
        { cache: "no-store" },
      );
      if (res.ok) {
        const data = (await res.json()) as { unread_count: number };
        setUnreadCount(data.unread_count ?? 0);
      }
    } catch (error: unknown) {
      console.error("NotificationBell: unread-count fetch failed", error);
    }
  }, [storeId]);

  useEffect(() => {
    fetchUnreadCount();
    const interval = setInterval(fetchUnreadCount, 30_000);
    return () => clearInterval(interval);
  }, [fetchUnreadCount]);

  // Fetch notifications when dropdown opens
  async function handleOpen() {
    setOpen((prev) => !prev);
    if (!open) {
      setLoading(true);
      try {
        const res = await fetch(
          `/api/notifications?storeId=${storeId}`,
          { cache: "no-store" },
        );
        if (res.ok) {
          const data = (await res.json()) as { notifications: NotificationItem[] };
          setNotifications(data.notifications ?? []);
        }
      } catch (error: unknown) {
        console.error("NotificationBell: list fetch failed", error);
      } finally {
        setLoading(false);
      }
    }
  }

  async function handleMarkAllRead() {
    try {
      const res = await fetch(`/api/notifications/read-all?storeId=${storeId}`, {
        method: "PATCH",
      });
      if (!res.ok) {
        console.error("NotificationBell: mark-all-read failed", res.status);
        return;
      }
      setUnreadCount(0);
      setNotifications((prev) =>
        prev.map((n) => ({ ...n, is_read: true })),
      );
    } catch (error: unknown) {
      console.error("NotificationBell: mark-all-read threw", error);
    }
  }

  // Close on outside click
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (
        dropdownRef.current &&
        !dropdownRef.current.contains(e.target as Node)
      ) {
        setOpen(false);
      }
    }
    if (open) {
      document.addEventListener("mousedown", handleClickOutside);
    }
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, [open]);

  // Close on Escape key
  useEffect(() => {
    function handleEscape(e: KeyboardEvent) {
      if (e.key === "Escape") setOpen(false);
    }
    if (open) {
      document.addEventListener("keydown", handleEscape);
    }
    return () => document.removeEventListener("keydown", handleEscape);
  }, [open]);

  return (
    <div className="relative" ref={dropdownRef}>
      <button
        type="button"
        onClick={handleOpen}
        className="relative inline-flex h-11 w-11 items-center justify-center rounded-md text-foreground-secondary transition-colors hover:bg-paper-100 hover:text-foreground"
        aria-label={`Notifications${unreadCount > 0 ? ` (${unreadCount} unread)` : ""}`}
      >
        <Bell className="h-4 w-4" aria-hidden="true" />
        {unreadCount > 0 && (
          <span className="absolute right-1.5 top-1.5 h-2 w-2 rounded-full bg-[color:var(--signal)]" />
        )}
      </button>

      {open && (
        <div className="absolute right-0 left-auto top-full z-50 mt-2 w-[calc(100vw-2rem)] sm:w-80 max-h-96 overflow-y-auto rounded-[6px] border border-border bg-[color:var(--background-elevated)] shadow-lg">
          <div className="flex items-center justify-between border-b border-border px-4 py-3">
            <p className="text-sm font-medium text-foreground">Notifications</p>
            {unreadCount > 0 && (
              <button
                type="button"
                onClick={handleMarkAllRead}
                className="inline-flex items-center gap-1 text-xs font-medium text-[color:var(--moss-700)] hover:underline"
              >
                <CheckCheck className="h-3 w-3" aria-hidden="true" />
                Mark all read
              </button>
            )}
          </div>

          {loading ? (
            <div className="px-4 py-8 text-center text-sm text-foreground-secondary">
              Loading...
            </div>
          ) : notifications.length === 0 ? (
            <div className="px-4 py-8 text-center text-sm text-foreground-secondary">
              No notifications yet.
            </div>
          ) : (
            <ul className="divide-y divide-border">
              {notifications.map((n) => {
                const Icon = TYPE_ICONS[n.type] ?? Bell;
                return (
                  <li
                    key={n.id}
                    className={`flex gap-3 px-4 py-3 ${!n.is_read ? "bg-[color:var(--paper-200)]/50" : ""}`}
                  >
                    <Icon
                      className="mt-0.5 h-4 w-4 shrink-0 text-foreground-secondary"
                      aria-hidden="true"
                    />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-start justify-between gap-2">
                        <p className="text-sm font-medium text-foreground truncate">
                          {n.title}
                        </p>
                        {!n.is_read && (
                          <span className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-[color:var(--moss-700)]" />
                        )}
                      </div>
                      {n.message && (
                        <p className="mt-0.5 text-xs text-foreground-secondary line-clamp-2">
                          {n.message}
                        </p>
                      )}
                      <p className="mt-1 text-[10px] text-foreground-tertiary">
                        {formatDistanceToNow(new Date(n.created_at), { addSuffix: true })}
                      </p>
                    </div>
                  </li>
                );
              })}
            </ul>
          )}

          <div className="border-t border-border px-4 py-2">
            <Link
              href="/settings/notifications"
              onClick={() => setOpen(false)}
              className="inline-flex items-center gap-1 text-xs font-medium text-[color:var(--moss-700)] hover:underline"
            >
              <Settings className="h-3 w-3" aria-hidden="true" />
              Notification settings
            </Link>
          </div>
        </div>
      )}
    </div>
  );
}
