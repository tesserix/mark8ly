"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { Input, Label } from "@tesserix/web";

import type { InvitationVerifyResult } from "@/lib/api/platform-api";
import { getGoogleCredential } from "@/lib/gip/google-gsi";
import {
  GIPError,
  signInWithGoogle,
  signInWithPassword,
  signUp,
} from "@/lib/gip/signup";
import { acceptInvite } from "@/app/accept-invite/actions";

type Mode = "signin" | "create";

interface AcceptInviteFormProps {
  token: string;
  invitation: InvitationVerifyResult;
}

export function AcceptInviteForm({
  token,
  invitation,
}: AcceptInviteFormProps) {
  const router = useRouter();
  const [mode, setMode] = useState<Mode>("signin");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();
  const [googlePending, setGooglePending] = useState(false);

  const disabled = pending || googlePending;

  function complete(
    idToken: string,
    uid: string,
    verifiedEmail: string,
  ) {
    startTransition(async () => {
      const result = await acceptInvite({
        token,
        idToken,
        uid,
        verifiedEmail,
      });
      if (!result.ok) {
        setError(result.message);
        return;
      }
      setSuccess("Invite accepted. Opening your store…");
      router.push("/dashboard");
      router.refresh();
    });
  }

  function handlePasswordFlow(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setSuccess(null);

    if (password.length < 8) {
      setError("Password must be at least 8 characters.");
      return;
    }

    if (mode === "create" && password !== confirmPassword) {
      setError("Passwords do not match.");
      return;
    }

    startTransition(async () => {
      try {
        const auth =
          mode === "create"
            ? await signUp(invitation.email, password)
            : await signInWithPassword(invitation.email, password);

        complete(auth.idToken, auth.uid, invitation.email);
      } catch (err) {
        setError(describeAuthError(err, mode));
      }
    });
  }

  async function handleGoogle() {
    setError(null);
    setSuccess(null);
    setGooglePending(true);
    try {
      const { credential } = await getGoogleCredential();
      const auth = await signInWithGoogle(credential);
      const googleEmail = decodeJwtEmail(auth.idToken);
      if (
        googleEmail &&
        googleEmail.toLowerCase() !== invitation.email.toLowerCase()
      ) {
        setError(
          `This Google account (${googleEmail}) does not match the invite email (${invitation.email}).`,
        );
        return;
      }
      complete(auth.idToken, auth.uid, invitation.email);
    } catch (err) {
      setError(describeGoogleError(err));
    } finally {
      setGooglePending(false);
    }
  }

  return (
    <div className="admin-surface overflow-hidden rounded-[2rem]">
      <div className="border-b admin-soft-rule bg-[linear-gradient(180deg,rgba(248,241,230,0.92),rgba(255,252,248,0.76))] px-6 pb-6 pt-7 sm:px-8">
        <div className="flex flex-wrap items-center gap-3">
          <span className="rounded-full border border-border/80 bg-white/80 px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.16em] text-[var(--app-brown)]">
            {invitation.role}
          </span>
          <span className="rounded-full border border-border/80 bg-white/80 px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.16em] text-muted-foreground">
            {invitation.tenant_slug}.mark8ly.com
          </span>
        </div>
        <p className="mt-4 text-sm leading-6 text-muted-foreground">
          Use <strong className="font-medium text-foreground">{invitation.email}</strong>{" "}
          to accept this invite. If you already have a Mark8ly account, sign in.
          Otherwise create one now and we&apos;ll attach it to the store.
        </p>
      </div>

      <div className="space-y-6 px-6 py-6 sm:px-8 sm:py-8">
        <div className="grid gap-3 sm:grid-cols-2">
          {modeOptions.map((option) => {
            const selected = option.value === mode;
            return (
              <button
                key={option.value}
                type="button"
                onClick={() => {
                  setMode(option.value);
                  setError(null);
                  setSuccess(null);
                }}
                className={`rounded-[1.25rem] border px-4 py-4 text-left transition-[border-color,background-color,box-shadow] ${
                  selected
                    ? "border-[rgba(138,100,64,0.4)] bg-[rgba(248,241,230,0.78)] shadow-[0_14px_30px_rgba(76,52,24,0.08)]"
                    : "border-border/70 bg-white/62 hover:border-border hover:bg-white/78"
                }`}
              >
                <p className="text-sm font-semibold text-foreground">{option.title}</p>
                <p className="mt-1 text-sm leading-6 text-muted-foreground">
                  {option.body}
                </p>
              </button>
            );
          })}
        </div>

        <form onSubmit={handlePasswordFlow} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="invite-email">Email</Label>
            <Input
              id="invite-email"
              type="email"
              value={invitation.email}
              readOnly
              disabled
              className="bg-white/72"
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="invite-password">
              {mode === "create" ? "Create password" : "Password"}
            </Label>
            <Input
              id="invite-password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="At least 8 characters"
              minLength={8}
              required
              disabled={disabled}
              autoComplete={mode === "create" ? "new-password" : "current-password"}
              className="bg-white/82"
            />
          </div>

          {mode === "create" && (
            <div className="space-y-2">
              <Label htmlFor="invite-confirm-password">Confirm password</Label>
              <Input
                id="invite-confirm-password"
                type="password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                placeholder="Repeat password"
                minLength={8}
                required
                disabled={disabled}
                autoComplete="new-password"
                className="bg-white/82"
              />
            </div>
          )}

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

          <div className="flex flex-col gap-3 pt-2">
            <button
              type="submit"
              disabled={disabled}
              className="inline-flex w-full items-center justify-center rounded-xl bg-primary px-6 py-3.5 text-sm font-medium text-primary-foreground shadow-[0_14px_30px_rgba(31,30,28,0.18)] transition-[transform,box-shadow,opacity] hover:-translate-y-0.5 hover:opacity-95 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {pending
                ? mode === "create"
                  ? "Creating account…"
                  : "Signing in…"
                : mode === "create"
                  ? "Create account and join store"
                  : "Sign in and accept invite"}
            </button>

            <div className="relative py-1">
              <div className="absolute inset-0 flex items-center" aria-hidden>
                <div className="w-full border-t admin-soft-rule" />
              </div>
              <div className="relative flex justify-center">
                <span className="bg-[rgba(255,253,249,0.96)] px-3 text-xs uppercase tracking-wider text-muted-foreground">
                  or
                </span>
              </div>
            </div>

            <button
              type="button"
              onClick={handleGoogle}
              disabled={disabled}
              className="inline-flex w-full items-center justify-center gap-3 rounded-xl border border-border/90 bg-white/82 px-6 py-3 text-sm font-medium text-foreground transition-[background-color,border-color] hover:bg-white disabled:cursor-not-allowed disabled:opacity-50"
            >
              <GoogleMark />
              {googlePending ? "Opening Google…" : "Continue with Google"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

const modeOptions: Array<{ value: Mode; title: string; body: string }> = [
  {
    value: "signin",
    title: "I already have an account",
    body: "Sign in with the invite email to join this store with your existing account.",
  },
  {
    value: "create",
    title: "Create a new account",
    body: "Set a password now if this is your first time joining Mark8ly.",
  },
];

function describeAuthError(err: unknown, mode: Mode): string {
  if (err instanceof GIPError) {
    if (err.code === "invalid_credentials") {
      return "Email or password is incorrect.";
    }
    if (err.code === "weak_password") {
      return "Password must be at least 8 characters.";
    }
    if (/EMAIL_EXISTS/.test(err.message) && mode === "create") {
      return "An account already exists for this email. Try signing in instead.";
    }
    return err.message;
  }
  return err instanceof Error ? err.message : "Something went wrong.";
}

function describeGoogleError(err: unknown): string {
  if (err instanceof Error) {
    return `Google sign-in failed: ${err.message}`;
  }
  return "Google sign-in failed.";
}

function decodeJwtEmail(token: string): string | null {
  try {
    const [, payload] = token.split(".");
    if (!payload) return null;
    const padded = payload.padEnd(
      payload.length + ((4 - (payload.length % 4)) % 4),
      "=",
    );
    const json = atob(padded.replace(/-/g, "+").replace(/_/g, "/"));
    const claims = JSON.parse(json) as { email?: string };
    return claims.email ?? null;
  } catch {
    return null;
  }
}

function GoogleMark() {
  return (
    <svg width="18" height="18" viewBox="0 0 18 18" aria-hidden>
      <path
        fill="#4285F4"
        d="M17.64 9.2c0-.637-.057-1.251-.164-1.84H9v3.481h4.844a4.14 4.14 0 0 1-1.796 2.716v2.258h2.908c1.702-1.567 2.684-3.874 2.684-6.615z"
      />
      <path
        fill="#34A853"
        d="M9 18c2.43 0 4.467-.806 5.956-2.184l-2.908-2.258c-.806.54-1.837.86-3.048.86-2.344 0-4.328-1.584-5.036-3.711H.957v2.332A8.997 8.997 0 0 0 9 18z"
      />
      <path
        fill="#FBBC05"
        d="M3.964 10.707A5.41 5.41 0 0 1 3.682 9c0-.593.102-1.17.282-1.707V4.961H.957A8.997 8.997 0 0 0 0 9c0 1.452.348 2.827.957 4.039l3.007-2.332z"
      />
      <path
        fill="#EA4335"
        d="M9 3.58c1.321 0 2.508.454 3.44 1.345l2.582-2.58C13.463.891 11.426 0 9 0A8.997 8.997 0 0 0 .957 4.961L3.964 7.293C4.672 5.166 6.656 3.58 9 3.58z"
      />
    </svg>
  );
}
