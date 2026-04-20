"use client";

import { useRef, useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { formatDistanceToNow } from "date-fns";
import { Shield, Monitor, Trash2, Upload, User as UserIcon } from "lucide-react";

import type { AccountProfile, AccountSession } from "@/lib/api/settings-tier2-api";
import {
  updateProfile,
  createAvatarUploadURL,
  enrollMFA,
  confirmMFA,
  disableMFAAction,
  revokeSessionAction,
  deleteAccountAction,
} from "@/app/(admin)/settings/actions";
import { useToast } from "@/components/feedback/Toaster";

interface AccountSettingsClientProps {
  profile: AccountProfile | null;
  sessionEmail: string;
  sessions: AccountSession[];
  editable: boolean;
}

export function AccountSettingsClient({
  profile,
  sessionEmail,
  sessions,
  editable,
}: AccountSettingsClientProps) {
  return (
    <div className="space-y-10">
      <ProfileSection
        profile={profile}
        sessionEmail={sessionEmail}
        editable={editable}
      />
      <hr className="border-border" />
      <MFASection mfaEnabled={profile?.mfa_enabled ?? false} editable={editable} />
      <hr className="border-border" />
      <SessionsSection sessions={sessions} />
      <hr className="border-border" />
      <DangerZone editable={editable} />
    </div>
  );
}

// ─── Profile Form ─────────────────────────────────────────────────────

function ProfileSection({
  profile,
  sessionEmail,
  editable,
}: {
  profile: AccountProfile | null;
  sessionEmail: string;
  editable: boolean;
}) {
  const [name, setName] = useState(profile?.name ?? "");
  const [phone, setPhone] = useState(profile?.phone ?? "");
  const [avatarUrl, setAvatarUrl] = useState(profile?.avatar_url ?? "");
  const [avatarUploading, setAvatarUploading] = useState(false);
  const email = profile?.email || sessionEmail || "";
  const [isPending, startTransition] = useTransition();
  const router = useRouter();
  const { toast } = useToast();
  const fileInputRef = useRef<HTMLInputElement | null>(null);

  function handleSave() {
    startTransition(async () => {
      const result = await updateProfile({
        name: name.trim(),
        phone: phone.trim(),
        avatar_url: avatarUrl.trim(),
      });
      if (!result.ok) {
        toast.error("Couldn't save profile", result.message);
      } else {
        toast.success("Profile updated");
        router.refresh();
      }
    });
  }

  async function persistAvatar(url: string) {
    const result = await updateProfile({ avatar_url: url });
    if (!result.ok) {
      toast.error("Couldn't save picture", result.message);
      return false;
    }
    router.refresh();
    return true;
  }

  async function handleAvatarFile(file: File) {
    if (!["image/png", "image/jpeg", "image/webp"].includes(file.type)) {
      toast.error("Unsupported file type", "Use PNG, JPEG, or WebP.");
      return;
    }
    if (file.size > 5 * 1024 * 1024) {
      toast.error("Image too large", "Please upload an image under 5 MB.");
      return;
    }
    setAvatarUploading(true);
    try {
      const signed = await createAvatarUploadURL({
        filename: file.name,
        content_type: file.type,
      });
      if (!signed.ok) {
        toast.error("Couldn't prepare upload", signed.message);
        return;
      }
      const put = await fetch(signed.data.upload_url, {
        method: "PUT",
        headers: { "Content-Type": file.type },
        body: file,
      });
      if (!put.ok) {
        toast.error("Upload failed", `Storage returned ${put.status}.`);
        return;
      }
      setAvatarUrl(signed.data.public_url);
      const saved = await persistAvatar(signed.data.public_url);
      if (saved) toast.success("Profile picture updated");
    } catch (e) {
      toast.error("Upload failed", e instanceof Error ? e.message : "Please try again.");
    } finally {
      setAvatarUploading(false);
    }
  }

  async function handleRemoveAvatar() {
    setAvatarUrl("");
    const saved = await persistAvatar("");
    if (saved) toast.success("Profile picture removed");
  }

  return (
    <section className="space-y-6">
      <div className="space-y-1">
        <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium tracking-tight text-foreground">
          Profile
        </h2>
        <p className="text-sm text-foreground-secondary">
          Your personal account details.
        </p>
      </div>

      <div className="flex items-start gap-6">
        <div className="relative h-20 w-20 shrink-0 overflow-hidden rounded-full border border-border bg-[color:var(--paper-200)]">
          {avatarUrl ? (
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={avatarUrl}
              alt="Profile picture"
              className="h-full w-full object-cover"
            />
          ) : (
            <div className="flex h-full w-full items-center justify-center text-foreground-tertiary">
              <UserIcon className="h-8 w-8" aria-hidden="true" />
            </div>
          )}
        </div>
        <div className="space-y-2">
          <p className="text-sm font-medium text-foreground">Profile picture</p>
          <p className="text-xs text-foreground-secondary">
            PNG, JPEG, or WebP · up to 5 MB.
          </p>
          {editable && (
            <div className="flex items-center gap-3">
              <input
                ref={fileInputRef}
                type="file"
                accept="image/png,image/jpeg,image/webp"
                className="hidden"
                onChange={(e) => {
                  const f = e.target.files?.[0];
                  if (f) void handleAvatarFile(f);
                  e.target.value = "";
                }}
              />
              <button
                type="button"
                onClick={() => fileInputRef.current?.click()}
                disabled={avatarUploading || isPending}
                className="inline-flex h-9 items-center gap-2 rounded-[6px] border border-border bg-[color:var(--background-elevated)] px-3 text-sm font-medium text-foreground transition-colors hover:bg-[color:var(--paper-200)] disabled:opacity-50"
              >
                <Upload className="h-4 w-4" aria-hidden="true" />
                {avatarUploading ? "Uploading..." : "Upload new picture"}
              </button>
              {avatarUrl && (
                <button
                  type="button"
                  onClick={() => void handleRemoveAvatar()}
                  disabled={avatarUploading || isPending}
                  className="text-xs font-medium text-foreground-secondary hover:text-foreground disabled:opacity-50"
                >
                  Remove
                </button>
              )}
            </div>
          )}
        </div>
      </div>

      <div className="grid max-w-md gap-4">
        <div className="space-y-1.5">
          <label htmlFor="profile-name" className="text-sm font-medium text-foreground">
            Name
          </label>
          <input
            id="profile-name"
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            disabled={!editable || isPending}
            className="h-10 w-full rounded-[6px] border border-border bg-[color:var(--background-elevated)] px-3 text-sm text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)] disabled:opacity-50"
            placeholder="Your name"
          />
        </div>
        <div className="space-y-1.5">
          <label htmlFor="profile-email" className="text-sm font-medium text-foreground">
            Email
          </label>
          <input
            id="profile-email"
            type="email"
            value={email}
            readOnly
            disabled
            aria-readonly="true"
            className="h-10 w-full rounded-[6px] border border-border bg-[color:var(--background-elevated)] px-3 text-sm text-foreground placeholder:text-foreground-tertiary disabled:opacity-60 disabled:cursor-not-allowed"
            placeholder="you@example.com"
          />
        </div>
        <div className="space-y-1.5">
          <label htmlFor="profile-phone" className="text-sm font-medium text-foreground">
            Phone
          </label>
          <input
            id="profile-phone"
            type="tel"
            value={phone}
            onChange={(e) => setPhone(e.target.value)}
            disabled={!editable || isPending}
            className="h-10 w-full rounded-[6px] border border-border bg-[color:var(--background-elevated)] px-3 text-sm text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)] disabled:opacity-50"
            placeholder="+1 555 123 4567"
          />
        </div>
      </div>
      {editable && (
        <button
          type="button"
          onClick={handleSave}
          disabled={isPending || avatarUploading}
          className="h-10 rounded-[6px] bg-[color:var(--ink-900)] px-5 text-sm font-medium text-white transition-colors hover:bg-[color:var(--ink-900)]/90 disabled:opacity-50"
        >
          {isPending ? "Saving..." : "Save changes"}
        </button>
      )}
    </section>
  );
}

// ─── MFA Section ──────────────────────────────────────────────────────

function MFASection({
  mfaEnabled,
  editable,
}: {
  mfaEnabled: boolean;
  editable: boolean;
}) {
  const [isPending, startTransition] = useTransition();
  const [qrUrl, setQrUrl] = useState<string | null>(null);
  const [secret, setSecret] = useState<string | null>(null);
  const [code, setCode] = useState("");
  const router = useRouter();
  const { toast } = useToast();

  function handleEnroll() {
    startTransition(async () => {
      const result = await enrollMFA();
      if (!result.ok) {
        toast.error("Couldn't start MFA setup", result.message);
        return;
      }
      setQrUrl(result.data.qr_code_url);
      setSecret(result.data.secret);
    });
  }

  function handleCancelEnroll() {
    setQrUrl(null);
    setSecret(null);
    setCode("");
  }

  function handleConfirm() {
    startTransition(async () => {
      const result = await confirmMFA(code);
      if (!result.ok) {
        toast.error("Couldn't verify code", result.message);
        return;
      }
      toast.success("Two-factor authentication enabled");
      setQrUrl(null);
      setSecret(null);
      setCode("");
      router.refresh();
    });
  }

  function handleDisable() {
    startTransition(async () => {
      const result = await disableMFAAction();
      if (!result.ok) {
        toast.error("Couldn't disable MFA", result.message);
        return;
      }
      toast.success("Two-factor authentication disabled");
      router.refresh();
    });
  }

  const enrolling = qrUrl !== null;

  return (
    <section className="space-y-6">
      <div className="flex items-start gap-4">
        <Shield className="mt-0.5 h-5 w-5 text-foreground-secondary" aria-hidden="true" />
        <div className="flex-1 space-y-1">
          <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium tracking-tight text-foreground">
            Two-factor authentication
          </h2>
          <p className="text-sm text-foreground-secondary">
            Add an extra layer of security to your account.
          </p>
        </div>
        <span
          className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${
            mfaEnabled
              ? "bg-[color:var(--moss-700)]/10 text-[color:var(--moss-700)]"
              : "bg-[color:var(--ink-900)]/10 text-[color:var(--ink-900)]/60"
          }`}
        >
          {mfaEnabled ? "Enabled" : "Disabled"}
        </span>
      </div>

      {enrolling && (
        <div className="max-w-md space-y-4 rounded-[6px] border border-border bg-[color:var(--background-elevated)] p-5">
          <div className="space-y-2">
            <p className="text-sm font-medium text-foreground">
              1. Scan this QR code with your authenticator app
            </p>
            <p className="text-xs text-foreground-secondary">
              Google Authenticator, 1Password, Authy — any TOTP-compatible app works.
            </p>
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img
              src={qrUrl!}
              alt="MFA QR code"
              className="h-48 w-48 rounded-[6px] border border-border bg-background-elevated"
            />
            {secret && (
              <details className="text-xs text-foreground-secondary">
                <summary className="cursor-pointer">Can&rsquo;t scan? Show setup key</summary>
                <code className="mt-2 block break-all rounded-[4px] bg-[color:var(--paper-200)] px-2 py-1 font-mono">
                  {secret}
                </code>
              </details>
            )}
          </div>

          <div className="space-y-2">
            <label
              htmlFor="mfa-code"
              className="block text-sm font-medium text-foreground"
            >
              2. Enter the 6-digit code from your app
            </label>
            <input
              id="mfa-code"
              type="text"
              inputMode="numeric"
              autoComplete="one-time-code"
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
              disabled={isPending}
              className="h-10 w-40 rounded-[6px] border border-border bg-background-elevated px-3 font-mono text-lg tracking-[0.4em] text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[color:var(--moss-700)] disabled:opacity-50"
              placeholder="000000"
            />
          </div>

          <div className="flex gap-3">
            <button
              type="button"
              onClick={handleConfirm}
              disabled={isPending || code.length !== 6}
              className="h-10 rounded-[6px] bg-[color:var(--ink-900)] px-5 text-sm font-medium text-white transition-colors hover:bg-[color:var(--ink-900)]/90 disabled:opacity-50"
            >
              {isPending ? "Verifying..." : "Verify & enable"}
            </button>
            <button
              type="button"
              onClick={handleCancelEnroll}
              disabled={isPending}
              className="h-10 rounded-[6px] border border-border bg-[color:var(--background-elevated)] px-5 text-sm font-medium text-foreground transition-colors hover:bg-[color:var(--paper-200)] disabled:opacity-50"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {editable && !enrolling && (
        <button
          type="button"
          onClick={mfaEnabled ? handleDisable : handleEnroll}
          disabled={isPending}
          className={`h-10 rounded-[6px] px-5 text-sm font-medium transition-colors disabled:opacity-50 ${
            mfaEnabled
              ? "border border-border bg-[color:var(--background-elevated)] text-foreground hover:bg-[color:var(--paper-200)]"
              : "bg-[color:var(--ink-900)] text-white hover:bg-[color:var(--ink-900)]/90"
          }`}
        >
          {isPending
            ? "Processing..."
            : mfaEnabled
              ? "Disable MFA"
              : "Enable MFA"}
        </button>
      )}
    </section>
  );
}

// ─── Active Sessions ──────────────────────────────────────────────────

function SessionsSection({ sessions }: { sessions: AccountSession[] }) {
  const [isPending, startTransition] = useTransition();
  const router = useRouter();
  const { toast } = useToast();

  function handleRevoke(sessionId: string) {
    startTransition(async () => {
      const result = await revokeSessionAction(sessionId);
      if (!result.ok) {
        toast.error("Couldn't revoke session", result.message);
      } else {
        toast.success("Session revoked");
        router.refresh();
      }
    });
  }

  return (
    <section className="space-y-6">
      <div className="flex items-start gap-4">
        <Monitor className="mt-0.5 h-5 w-5 text-foreground-secondary" aria-hidden="true" />
        <div className="space-y-1">
          <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium tracking-tight text-foreground">
            Active sessions
          </h2>
          <p className="text-sm text-foreground-secondary">
            Devices where your account is currently signed in.
          </p>
        </div>
      </div>
      {sessions.length === 0 ? (
        <p className="text-sm text-foreground-secondary">No active sessions found.</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border text-left">
                <th className="pb-3 pr-4 font-medium text-foreground-secondary">Device</th>
                <th className="pb-3 pr-4 font-medium text-foreground-secondary">IP address</th>
                <th className="pb-3 pr-4 font-medium text-foreground-secondary">Last active</th>
                <th className="pb-3 font-medium text-foreground-secondary" />
              </tr>
            </thead>
            <tbody>
              {sessions.map((s) => (
                <tr key={s.id} className="border-b border-border">
                  <td className="py-3 pr-4 text-foreground">
                    {s.device}
                    {s.current && (
                      <span className="ml-2 inline-flex items-center rounded-full bg-[color:var(--moss-700)]/10 px-2 py-0.5 text-xs font-medium text-[color:var(--moss-700)]">
                        Current
                      </span>
                    )}
                  </td>
                  <td className="py-3 pr-4 font-mono text-xs text-foreground-secondary">{s.ip_address}</td>
                  <td className="py-3 pr-4 text-foreground-secondary">
                    {formatDistanceToNow(new Date(s.last_active), { addSuffix: true })}
                  </td>
                  <td className="py-3 text-right">
                    {!s.current && (
                      <button
                        type="button"
                        onClick={() => handleRevoke(s.id)}
                        disabled={isPending}
                        className="text-xs font-medium text-[color:var(--signal)] hover:underline disabled:opacity-50"
                      >
                        Revoke
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

// ─── Danger Zone ──────────────────────────────────────────────────────

function DangerZone({ editable }: { editable: boolean }) {
  const [showConfirm, setShowConfirm] = useState(false);
  const [confirmation, setConfirmation] = useState("");
  const [isPending, startTransition] = useTransition();
  const { toast } = useToast();

  function handleDelete() {
    startTransition(async () => {
      const result = await deleteAccountAction(confirmation);
      if (!result.ok) {
        toast.error("Couldn't delete account", result.message);
      } else {
        window.location.href = "/logout";
      }
    });
  }

  if (!editable) return null;

  return (
    <section className="space-y-4">
      <h2 className="font-[family-name:var(--font-source-serif),'Source_Serif_4',serif] text-2xl font-medium tracking-tight text-[color:var(--danger)]">
        Danger zone
      </h2>
      <div className="rounded-[6px] border border-[color:var(--danger)]/30 p-6 space-y-4">
        <div className="flex items-start gap-3">
          <Trash2 className="mt-0.5 h-5 w-5 text-[color:var(--danger)]" aria-hidden="true" />
          <div className="space-y-1">
            <p className="text-sm font-medium text-foreground">Reset my profile</p>
            <p className="text-sm text-foreground-secondary">
              Clears your name, phone, profile picture, and two-factor
              enrolment, and signs you out of every device. Your store
              data is preserved — sign in again to pick up where you
              left off. To remove your store entirely, contact support.
            </p>
          </div>
        </div>
        {!showConfirm ? (
          <button
            type="button"
            onClick={() => setShowConfirm(true)}
            className="h-10 rounded-[6px] bg-[color:var(--ink-900)] px-5 text-sm font-medium text-white transition-colors hover:bg-[color:var(--ink-900)]/90"
          >
            Reset my profile
          </button>
        ) : (
          <div className="space-y-3">
            <p className="text-sm text-foreground">
              Type <strong>reset my profile</strong> to confirm:
            </p>
            <input
              type="text"
              value={confirmation}
              onChange={(e) => setConfirmation(e.target.value)}
              disabled={isPending}
              className="h-10 w-full max-w-xs rounded-[6px] border border-[color:var(--danger)]/30 bg-[color:var(--background-elevated)] px-3 text-sm text-foreground placeholder:text-foreground-tertiary focus:border-[color:var(--danger)] focus:outline-none focus:ring-1 focus:ring-[color:var(--danger)] disabled:opacity-50"
              placeholder="reset my profile"
            />
            <div className="flex gap-3">
              <button
                type="button"
                onClick={handleDelete}
                disabled={isPending || confirmation !== "reset my profile"}
                className="h-10 rounded-[6px] bg-[color:var(--danger)] px-5 text-sm font-medium text-white transition-colors hover:bg-[color:var(--danger)]/90 disabled:opacity-50"
              >
                {isPending ? "Resetting..." : "Reset and sign out"}
              </button>
              <button
                type="button"
                onClick={() => {
                  setShowConfirm(false);
                  setConfirmation("");
                }}
                className="h-10 rounded-[6px] border border-border bg-[color:var(--background-elevated)] px-5 text-sm font-medium text-foreground transition-colors hover:bg-[color:var(--paper-200)]"
              >
                Cancel
              </button>
            </div>
          </div>
        )}
      </div>
    </section>
  );
}
