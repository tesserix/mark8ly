/**
 * PlatformAnnouncementBanner — surfaces active announcements published
 * by the Tesserix platform team. Reads via the local /api/platform-
 * announcements proxy (which holds the bearer token server-side and
 * forwards to tesserix-home).
 *
 * Behavior:
 *   - Polls every 5 minutes via React Query so a freshly-published
 *     announcement appears within minutes.
 *   - Renders the highest-severity active announcement that the user
 *     hasn't dismissed (dismissals are stored per-id in localStorage).
 *   - Severity → tone mapping:
 *       incident     → danger
 *       warning      → warning
 *       maintenance  → warning
 *       info         → info
 *
 * Mounted unconditionally in AdminShell — visible on every authenticated
 * route, not gated on currentStoreId.
 */
"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { BannerShell, type BannerTone } from "./BannerShell";

interface Announcement {
  id: string;
  title: string;
  body: string;
  severity: string;
  starts_at: string;
  ends_at: string | null;
}

const SEVERITY_TO_TONE: Record<string, BannerTone> = {
  info: "info",
  warning: "warning",
  maintenance: "warning",
  incident: "danger",
};

const SEVERITY_RANK: Record<string, number> = {
  incident: 0,
  warning: 1,
  maintenance: 2,
  info: 3,
};

const DISMISSED_KEY = "tesserix-platform-announcement-dismissed";
const POLL_INTERVAL_MS = 5 * 60 * 1000;

function readDismissed(): Record<string, true> {
  if (typeof window === "undefined") return {};
  try {
    const raw = window.localStorage.getItem(DISMISSED_KEY);
    return raw ? (JSON.parse(raw) as Record<string, true>) : {};
  } catch {
    return {};
  }
}

function writeDismissed(state: Record<string, true>): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(DISMISSED_KEY, JSON.stringify(state));
  } catch {
    /* localStorage unavailable — silently no-op */
  }
}

export function PlatformAnnouncementBanner() {
  const { data } = useQuery<{ rows: Announcement[] }>({
    queryKey: ["platform-announcements"],
    queryFn: async () => {
      const res = await fetch("/api/platform-announcements", {
        credentials: "include",
      });
      if (!res.ok) return { rows: [] };
      return (await res.json()) as { rows: Announcement[] };
    },
    refetchInterval: POLL_INTERVAL_MS,
    staleTime: POLL_INTERVAL_MS,
  });

  const [dismissed, setDismissed] = React.useState<Record<string, true>>({});
  React.useEffect(() => {
    setDismissed(readDismissed());
  }, []);

  const visible = React.useMemo(() => {
    const rows = data?.rows ?? [];
    const undismissed = rows.filter((a) => !dismissed[a.id]);
    if (undismissed.length === 0) return null;
    return [...undismissed].sort(
      (a, b) =>
        (SEVERITY_RANK[a.severity] ?? 4) - (SEVERITY_RANK[b.severity] ?? 4),
    )[0];
  }, [data, dismissed]);

  if (!visible) return null;

  const tone = SEVERITY_TO_TONE[visible.severity] ?? "info";

  function handleDismiss() {
    if (!visible) return;
    const next = { ...dismissed, [visible.id]: true as const };
    writeDismissed(next);
    setDismissed(next);
  }

  return (
    <div data-testid="platform-announcement-banner">
      <BannerShell
        tone={tone}
        heading={visible.title}
        body={visible.body}
        dismissible
        onDismiss={handleDismiss}
      />
    </div>
  );
}
