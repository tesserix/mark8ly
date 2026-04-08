"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import {
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@tesserix/web";

import type {
  Invitation,
  InviteRole,
  Store,
} from "@/lib/api/platform-api";
import {
  inviteTeammate,
  revokeInvite,
} from "@/app/settings/team/actions";

interface TeamSettingsProps {
  ownerEmail: string;
  invitations: Invitation[];
  canInvite: boolean;
  // Phase R: every store under the current tenant. Powers the
  // "Tenant-wide" vs "Specific store" scope picker on the invite
  // form. When the tenant has a single store the store picker is
  // hidden — nothing meaningful to choose.
  stores: Store[];
}

type Scope = "tenant" | "store";

/**
 * Phase P — /settings/team client component.
 *
 * Renders the owner row + pending invitations table. Owners/admins
 * see an invite form inline; other roles see a read-only view.
 * No modal yet — the inline form is small enough that the extra
 * portal complexity isn't worth it for v1.
 */
export function TeamSettings({
  ownerEmail,
  invitations,
  canInvite,
  stores,
}: TeamSettingsProps) {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [scope, setScope] = useState<Scope>("tenant");
  const [storeId, setStoreId] = useState<string>(stores[0]?.id ?? "");
  const [role, setRole] = useState<InviteRole>("staff");
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  // Role options depend on scope. Tenant-wide: admin/staff/viewer
  // (matches the tenant DSL). Store-scoped: manager/staff/viewer
  // (matches the store DSL). When the scope flips we reset the
  // role to the safe "staff" default so we never ship a combo
  // that would fail the backend allowlist.
  const roleOptions: InviteRole[] =
    scope === "tenant"
      ? ["admin", "staff", "viewer"]
      : ["manager", "staff", "viewer"];

  function handleScopeChange(next: Scope) {
    setScope(next);
    setRole("staff");
  }

  function handleInvite(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSuccess(null);
    const trimmed = email.trim().toLowerCase();
    if (!trimmed.includes("@")) {
      setError("Please enter a valid email address.");
      return;
    }
    if (scope === "store" && !storeId) {
      setError("Please pick a store for the invite.");
      return;
    }
    startTransition(async () => {
      const result = await inviteTeammate({
        email: trimmed,
        role,
        storeId: scope === "store" ? storeId : undefined,
      });
      if (!result.ok) {
        setError(result.message);
        return;
      }
      setSuccess(`Invitation sent to ${trimmed}.`);
      setEmail("");
      router.refresh();
    });
  }

  function handleRevoke(id: string) {
    setError(null);
    setSuccess(null);
    startTransition(async () => {
      const result = await revokeInvite(id);
      if (!result.ok) {
        setError(result.message);
        return;
      }
      router.refresh();
    });
  }

  return (
    <div className="space-y-6">
      <section className="grid gap-4 lg:grid-cols-[minmax(0,1.05fr)_minmax(18rem,0.95fr)]">
        <div className="admin-panel space-y-4 rounded-[1.6rem] p-6">
          <h2 className="text-sm font-semibold uppercase tracking-[0.16em] text-muted-foreground">
            Owner
          </h2>
          <div className="rounded-[1.25rem] border border-border/60 bg-white/60 px-4 py-4">
            <div className="flex items-start justify-between gap-4">
              <div className="min-w-0">
                <p className="text-sm font-medium text-foreground">
                  {ownerEmail || "Unknown owner"}
                </p>
                <p className="mt-1 text-xs leading-5 text-muted-foreground">
                  Founder account. Contact support if ownership needs to move to
                  a different person.
                </p>
              </div>
              <span className="rounded-full border border-border/70 bg-muted/50 px-2.5 py-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">
                owner
              </span>
            </div>
          </div>
        </div>

        <div className="admin-panel space-y-4 rounded-[1.6rem] p-6">
          <h2 className="text-sm font-semibold uppercase tracking-[0.16em] text-muted-foreground">
            Roles
          </h2>
          <div className="space-y-3">
            {roleNotes.map((item) => (
              <div
                key={item.role}
                className="rounded-[1.15rem] border border-border/60 bg-white/56 px-4 py-3"
              >
                <div className="flex items-center justify-between gap-3">
                  <p className="text-sm font-medium text-foreground">{item.role}</p>
                  <span className="text-xs uppercase tracking-[0.14em] text-muted-foreground">
                    {item.scope}
                  </span>
                </div>
                <p className="mt-1 text-xs leading-5 text-muted-foreground">
                  {item.body}
                </p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {canInvite && (
        <section className="admin-panel space-y-4 rounded-[1.6rem] p-6">
          <h2 className="text-sm font-semibold uppercase tracking-[0.16em] text-muted-foreground">
            Invite teammate
          </h2>
          <p className="max-w-2xl text-sm leading-6 text-muted-foreground">
            Send an invitation by email and choose how much access the teammate
            should have once they join the store.
          </p>
          {stores.length > 1 && (
            <div
              data-testid="invite-scope"
              className="flex w-fit items-center gap-2 rounded-xl border border-border/70 bg-white/60 p-1"
            >
              <button
                type="button"
                onClick={() => handleScopeChange("tenant")}
                disabled={pending}
                className={`rounded-lg px-3 py-1.5 text-xs font-medium transition-colors ${
                  scope === "tenant"
                    ? "bg-primary text-primary-foreground shadow-sm"
                    : "text-muted-foreground hover:text-foreground"
                }`}
              >
                Tenant-wide
              </button>
              <button
                type="button"
                onClick={() => handleScopeChange("store")}
                disabled={pending}
                className={`rounded-lg px-3 py-1.5 text-xs font-medium transition-colors ${
                  scope === "store"
                    ? "bg-primary text-primary-foreground shadow-sm"
                    : "text-muted-foreground hover:text-foreground"
                }`}
              >
                Specific store
              </button>
            </div>
          )}
          <form
            onSubmit={handleInvite}
            className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_10rem_auto] sm:items-end"
          >
            <div className="space-y-2">
              <Label htmlFor="invite-email">Email</Label>
              <Input
                id="invite-email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="teammate@example.com"
                disabled={pending}
                required
                className="bg-white/82"
              />
            </div>
            {scope === "store" && (
              <div className="space-y-2 sm:col-span-3">
                <Label htmlFor="invite-store">Store</Label>
                <select
                  id="invite-store"
                  value={storeId}
                  onChange={(e) => setStoreId(e.target.value)}
                  disabled={pending}
                  data-testid="invite-store-select"
                  className="h-10 w-full rounded-xl border border-border bg-white/82 px-3 text-sm text-foreground shadow-sm focus:outline-none focus:ring-2 focus:ring-primary/20"
                >
                  {stores.map((s) => (
                    <option key={s.id} value={s.id}>
                      {s.name} ({s.slug}.mark8ly.com)
                    </option>
                  ))}
                </select>
              </div>
            )}
            <div className="space-y-2">
              <Label htmlFor="invite-role">Role</Label>
              <Select
                value={role}
                onValueChange={(value) => setRole(value as InviteRole)}
                disabled={pending}
              >
                <SelectTrigger
                  id="invite-role"
                  className="h-10 rounded-xl border-border bg-white/82 text-sm shadow-sm"
                >
                  <SelectValue placeholder="Choose a role" />
                </SelectTrigger>
                <SelectContent className="rounded-2xl border-border/80 bg-[rgba(255,252,248,0.98)] shadow-[0_24px_60px_rgba(76,52,24,0.14)]">
                  {roleOptions.map((r) => (
                    <SelectItem
                      key={r}
                      value={r}
                      className="rounded-xl focus:bg-primary focus:text-primary-foreground"
                    >
                      {r.charAt(0).toUpperCase() + r.slice(1)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <button
              type="submit"
              disabled={pending}
              className="inline-flex h-10 items-center justify-center rounded-xl bg-primary px-5 text-sm font-medium text-primary-foreground shadow-[0_14px_30px_rgba(31,30,28,0.18)] transition-[transform,box-shadow,opacity] hover:-translate-y-0.5 hover:opacity-95 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {pending ? "Sending..." : "Send invite"}
            </button>
          </form>
          {error && (
            <div
              role="alert"
              className="rounded-2xl border border-destructive/20 bg-destructive/5 px-4 py-3 text-sm text-destructive"
            >
              {error}
            </div>
          )}
          {success && (
            <div
              role="status"
              className="rounded-2xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-800"
            >
              {success}
            </div>
          )}
        </section>
      )}

      <section className="admin-panel space-y-4 rounded-[1.6rem] p-6">
        <h2 className="text-sm font-semibold uppercase tracking-[0.16em] text-muted-foreground">
          Pending invitations
        </h2>
        {invitations.length === 0 ? (
          <div className="rounded-[1.25rem] border border-dashed border-border/70 bg-white/45 px-4 py-5 text-sm text-muted-foreground">
            No pending invitations right now.
          </div>
        ) : (
          <ul className="divide-y divide-border/60">
            {invitations.map((inv) => (
              <li
                key={inv.id}
                className="flex items-center justify-between gap-4 py-4"
              >
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium text-foreground">
                    {inv.email}
                  </p>
                  <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                    <span className="rounded-full border border-border/70 bg-white/72 px-2 py-0.5 uppercase tracking-[0.14em]">
                      {inv.role}
                    </span>
                    <span>Invited {new Date(inv.created_at).toLocaleDateString()}</span>
                  </div>
                </div>
                {canInvite && (
                  <button
                    type="button"
                    onClick={() => handleRevoke(inv.id)}
                    disabled={pending}
                    className="rounded-xl border border-border px-3 py-1.5 text-xs font-medium text-muted-foreground transition-[border-color,color,background-color] hover:border-destructive hover:bg-destructive/5 hover:text-destructive disabled:cursor-not-allowed disabled:opacity-50"
                  >
                    Revoke
                  </button>
                )}
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}

const roleNotes = [
  {
    role: "Admin",
    scope: "Broad access",
    body: "Can manage settings, invite teammates, and help run the store alongside the owner.",
  },
  {
    role: "Staff",
    scope: "Operational",
    body: "Best for day-to-day catalog and order work without giving full control over the workspace.",
  },
  {
    role: "Viewer",
    scope: "Read only",
    body: "Can look around the store workspace without making changes.",
  },
];
