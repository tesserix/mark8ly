"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { Input } from "@tesserix/web";
import { Field } from "@repo/ui/field";

import type {
  Invitation,
  InviteRole,
  Store,
  TeamMember,
} from "@/lib/api/platform-api";
import {
  inviteTeammate,
  revokeInvite,
} from "@/app/settings/team/actions";

interface TeamSettingsProps {
  members: TeamMember[];
  invitations: Invitation[];
  canInvite: boolean;
  stores: Store[];
}

type Scope = "tenant" | "store";

/**
 * /settings/team client component — Paper · Ink · Moss.
 *
 * Three editorial sections separated by hairline rules:
 *   1. Team (owner + accepted invitations)
 *   2. Invite form (owner/admin only)
 *   3. Pending invitations
 *
 * The members list fetches from platform-api's
 * /internal/tenants/{id}/members which joins the tenants row (owner)
 * with accepted invitations rows. Source of truth for "who is on
 * this team right now" is the invitation record — every membership
 * in the current product flow goes through an invitation.
 */
export function TeamSettings({
  members,
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

  // Role options depend on scope. Tenant-wide: admin/staff/viewer.
  // Store-scoped: manager/staff/viewer. When the scope flips we reset
  // the role to the safe "staff" default so we never ship a combo
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
    <div className="space-y-16">
      {/* Members list */}
      <section>
        <div className="flex items-baseline justify-between border-b border-border-subtle pb-4">
          <div>
            <p className="eyebrow">Team</p>
            <h2 className="mt-1 font-serif text-2xl font-medium tracking-tight text-foreground">
              Who&rsquo;s on the team
            </h2>
          </div>
          <p className="text-xs text-foreground-tertiary">
            {members.length} {members.length === 1 ? "person" : "people"}
          </p>
        </div>

        {members.length === 0 ? (
          <p className="mt-6 text-sm text-foreground-tertiary">
            Just you so far. Invite a teammate below to start collaborating.
          </p>
        ) : (
          <ul className="divide-y divide-border-subtle">
            {members.map((m) => (
              <li
                key={`${m.kind}-${m.email}`}
                className="flex items-center justify-between gap-4 py-5"
              >
                <div className="min-w-0">
                  <p className="truncate text-base font-medium text-foreground">
                    {m.email}
                  </p>
                  <p className="mt-1 text-xs text-foreground-tertiary">
                    {m.kind === "owner"
                      ? "Founder account — contact support to move ownership"
                      : m.accepted_at
                        ? `Joined ${new Date(m.accepted_at).toLocaleDateString()}`
                        : "Joined via invitation"}
                  </p>
                </div>
                <span className="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground-tertiary">
                  {m.role}
                </span>
              </li>
            ))}
          </ul>
        )}
      </section>

      {/* Invite form */}
      {canInvite && (
        <section className="border-t border-border-subtle pt-10">
          <div className="pb-6">
            <p className="eyebrow">Invite</p>
            <h2 className="mt-1 font-serif text-2xl font-medium tracking-tight text-foreground">
              Add a teammate
            </h2>
            <p className="mt-2 max-w-xl text-sm leading-7 text-foreground-secondary">
              Send an invitation by email and choose how much access the
              teammate should have once they join.
            </p>
          </div>

          {stores.length > 1 && (
            <div
              data-testid="invite-scope"
              role="tablist"
              aria-label="Invite scope"
              className="mb-6 grid w-fit grid-cols-2 border border-border"
            >
              {([
                { value: "tenant", label: "Tenant-wide" },
                { value: "store", label: "Specific store" },
              ] as const).map((opt) => {
                const selected = scope === opt.value;
                return (
                  <button
                    key={opt.value}
                    type="button"
                    role="tab"
                    aria-selected={selected}
                    onClick={() => handleScopeChange(opt.value)}
                    disabled={pending}
                    className={`h-11 px-5 text-sm font-medium transition-colors ${
                      selected
                        ? "bg-primary text-primary-foreground"
                        : "bg-background-elevated text-foreground-secondary hover:text-foreground"
                    }`}
                  >
                    {opt.label}
                  </button>
                );
              })}
            </div>
          )}

          <form onSubmit={handleInvite} className="space-y-5" noValidate>
            <div className="grid gap-5 sm:grid-cols-[minmax(0,1fr)_12rem]">
              <Field id="invite-email" label="Email">
                <Input
                  id="invite-email"
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="teammate@example.com"
                  disabled={pending}
                  required
                  autoComplete="email"
                  spellCheck={false}
                />
              </Field>

              <Field id="invite-role" label="Role">
                <select
                  id="invite-role"
                  value={role}
                  onChange={(e) => setRole(e.target.value as InviteRole)}
                  disabled={pending}
                  className="h-11 w-full rounded-md border border-border bg-background-elevated px-3 text-sm text-foreground focus:border-ring focus:outline-none focus:ring-2 focus:ring-ring/20"
                >
                  {roleOptions.map((r) => (
                    <option key={r} value={r}>
                      {r.charAt(0).toUpperCase() + r.slice(1)}
                    </option>
                  ))}
                </select>
              </Field>
            </div>

            {scope === "store" && (
              <Field id="invite-store" label="Store">
                <select
                  id="invite-store"
                  value={storeId}
                  onChange={(e) => setStoreId(e.target.value)}
                  disabled={pending}
                  data-testid="invite-store-select"
                  className="h-11 w-full rounded-md border border-border bg-background-elevated px-3 text-sm text-foreground focus:border-ring focus:outline-none focus:ring-2 focus:ring-ring/20"
                >
                  {stores.map((s) => (
                    <option key={s.id} value={s.id}>
                      {s.name} ({s.slug}.mark8ly.com)
                    </option>
                  ))}
                </select>
              </Field>
            )}

            {error && (
              <p role="alert" className="text-sm text-danger">
                {error}
              </p>
            )}
            {success && (
              <p role="status" className="text-sm text-moss-700">
                {success}
              </p>
            )}

            <div>
              <button
                type="submit"
                disabled={pending}
                className="inline-flex h-12 items-center justify-center rounded-md bg-primary px-6 text-base font-medium text-primary-foreground hover:bg-primary-hover disabled:cursor-not-allowed disabled:bg-ink-600"
              >
                {pending ? "Sending…" : "Send invitation"}
              </button>
            </div>
          </form>
        </section>
      )}

      {/* Pending invitations */}
      <section className="border-t border-border-subtle pt-10">
        <div className="flex items-baseline justify-between border-b border-border-subtle pb-4">
          <div>
            <p className="eyebrow">Pending</p>
            <h2 className="mt-1 font-serif text-2xl font-medium tracking-tight text-foreground">
              Awaiting acceptance
            </h2>
          </div>
          <p className="text-xs text-foreground-tertiary">
            {invitations.length}{" "}
            {invitations.length === 1 ? "invitation" : "invitations"}
          </p>
        </div>

        {invitations.length === 0 ? (
          <p className="mt-6 text-sm text-foreground-tertiary">
            No pending invitations right now.
          </p>
        ) : (
          <ul className="divide-y divide-border-subtle">
            {invitations.map((inv) => (
              <li
                key={inv.id}
                className="flex items-center justify-between gap-4 py-5"
              >
                <div className="min-w-0">
                  <p className="truncate text-base font-medium text-foreground">
                    {inv.email}
                  </p>
                  <p className="mt-1 text-xs text-foreground-tertiary">
                    Invited {new Date(inv.created_at).toLocaleDateString()} ·{" "}
                    <span className="uppercase tracking-[0.14em]">
                      {inv.role}
                    </span>
                  </p>
                </div>
                {canInvite && (
                  <button
                    type="button"
                    onClick={() => handleRevoke(inv.id)}
                    disabled={pending}
                    className="inline-flex h-10 items-center rounded-md border border-border px-4 text-sm font-medium text-foreground-secondary hover:border-danger hover:text-danger disabled:cursor-not-allowed disabled:opacity-50"
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
