"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import {
  Input,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@tesserix/web";
import { Field } from "@repo/ui/field";
import { RoleBadge } from "@repo/ui/role-badge";

import type {
  Invitation,
  InviteRole,
  Store,
  TeamMember,
} from "@/lib/api/platform-api";
import {
  changeMemberRole,
  inviteTeammate,
  revokeInvite,
} from "@/app/settings/team/actions";

interface TeamSettingsProps {
  members: TeamMember[];
  invitations: Invitation[];
  canInvite: boolean;
  /**
   * The current user's role. Used to gate the inline role-change
   * dropdown on each member row — only owner/admin see it, and
   * admins can only change staff/viewer rows (backend enforces
   * the same rule, this is just UX).
   */
  currentRole?: string;
  /** The current user's email. Used to hide the role picker on
   *  their own row so they can't demote themselves by accident. */
  currentUserEmail?: string;
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
  currentRole,
  currentUserEmail,
  stores,
}: TeamSettingsProps) {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [roleChangePending, setRoleChangePending] = useState<string | null>(
    null,
  );
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

  function handleRoleChange(targetEmail: string, newRole: InviteRole) {
    setError(null);
    setSuccess(null);
    setRoleChangePending(targetEmail);
    startTransition(async () => {
      const result = await changeMemberRole({
        email: targetEmail,
        newRole,
      });
      setRoleChangePending(null);
      if (!result.ok) {
        setError(result.message);
        return;
      }
      setSuccess(`Updated ${targetEmail} to ${newRole}.`);
      router.refresh();
    });
  }

  /**
   * canEditMemberRole — per-row gate. Backend re-checks the same
   * rules; this is pure UX to keep the dropdown from rendering on
   * rows the user can't touch.
   *
   * Rules:
   *   - Owner row: never editable here (support-only)
   *   - Your own row: never editable (no self-demotion)
   *   - Owner (viewer): can edit everything else
   *   - Admin (viewer): can edit staff/viewer only, not other admins
   */
  function canEditMemberRole(m: TeamMember): boolean {
    if (m.kind === "owner") return false;
    if (currentUserEmail && m.email.toLowerCase() === currentUserEmail.toLowerCase()) {
      return false;
    }
    if (currentRole === "owner") return true;
    if (currentRole === "admin") {
      return m.role === "staff" || m.role === "viewer";
    }
    return false;
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
            {members.map((m) => {
              const editable = canEditMemberRole(m);
              const isPending = roleChangePending === m.email;
              const adminEditOptions: InviteRole[] =
                currentRole === "owner"
                  ? ["admin", "staff", "viewer"]
                  : ["staff", "viewer"];
              return (
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
                  {editable ? (
                    <div className="flex items-center gap-3">
                      {isPending && (
                        <span className="text-xs text-moss-700">Saving…</span>
                      )}
                      <div className="w-[9rem]">
                        <Select
                          value={m.role}
                          onValueChange={(value) =>
                            handleRoleChange(m.email, value as InviteRole)
                          }
                          disabled={pending}
                        >
                          <SelectTrigger
                            aria-label={`Change role for ${m.email}`}
                          >
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {adminEditOptions.map((r) => (
                              <SelectItem key={r} value={r}>
                                {r.charAt(0).toUpperCase() + r.slice(1)}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </div>
                    </div>
                  ) : (
                    <RoleBadge role={m.role} />
                  )}
                </li>
              );
            })}
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
                <Select
                  value={role}
                  onValueChange={(value) => setRole(value as InviteRole)}
                  disabled={pending}
                >
                  <SelectTrigger id="invite-role">
                    <SelectValue placeholder="Choose a role" />
                  </SelectTrigger>
                  <SelectContent>
                    {roleOptions.map((r) => (
                      <SelectItem key={r} value={r}>
                        {r.charAt(0).toUpperCase() + r.slice(1)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            </div>

            {scope === "store" && (
              <Field id="invite-store" label="Store">
                <Select
                  value={storeId}
                  onValueChange={setStoreId}
                  disabled={pending}
                >
                  <SelectTrigger
                    id="invite-store"
                    data-testid="invite-store-select"
                  >
                    <SelectValue placeholder="Pick a store" />
                  </SelectTrigger>
                  <SelectContent>
                    {stores.map((s) => (
                      <SelectItem key={s.id} value={s.id}>
                        {s.name} ({s.slug}.mark8ly.com)
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
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
                <div className="flex min-w-0 items-center gap-3">
                  <div className="min-w-0">
                    <p className="truncate text-base font-medium text-foreground">
                      {inv.email}
                    </p>
                    <p className="mt-1 text-xs text-foreground-tertiary">
                      Invited {new Date(inv.created_at).toLocaleDateString()}
                    </p>
                  </div>
                  <RoleBadge role={inv.role} />
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
