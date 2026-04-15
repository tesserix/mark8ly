"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { customerSignUp } from "@/app/create-account/actions";

interface GipConfig {
  apiKey: string;
  tenantId: string;
  projectId: string;
}

interface CreateAccountFormProps {
  gipConfig: GipConfig;
  storeSlug: string;
  returnUrl: string;
}

class GIPError extends Error {
  constructor(
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

async function signUpWithPassword(
  email: string,
  password: string,
  config: GipConfig,
): Promise<{ uid: string; idToken: string }> {
  if (!config.apiKey || !config.tenantId) {
    throw new GIPError(
      "config_missing",
      "Account creation is not configured for this store yet.",
    );
  }

  const url = `https://identitytoolkit.googleapis.com/v1/accounts:signUp?key=${config.apiKey}`;
  const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      email,
      password,
      returnSecureToken: true,
      tenantId: config.tenantId,
    }),
  });

  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as {
      error?: { message?: string };
    } | null;
    const code = body?.error?.message ?? "UNKNOWN";
    const friendly: Record<string, string> = {
      EMAIL_EXISTS: "An account with this email already exists. Try signing in instead.",
      WEAK_PASSWORD: "Password must be at least 6 characters.",
      INVALID_EMAIL: "Please enter a valid email address.",
      TOO_MANY_ATTEMPTS_TRY_LATER:
        "Too many attempts. Please wait a moment and try again.",
      OPERATION_NOT_ALLOWED:
        "Account creation is not enabled for this store yet.",
    };
    throw new GIPError(
      code,
      friendly[code] ?? "Account creation failed. Please try again.",
    );
  }

  const data = (await res.json()) as {
    localId: string;
    idToken: string;
  };
  return { uid: data.localId, idToken: data.idToken };
}

export function CreateAccountForm({
  gipConfig,
  storeSlug,
  returnUrl,
}: CreateAccountFormProps) {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [pending, startTransition] = useTransition();

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);

    if (!email.trim()) {
      setError("Email is required.");
      return;
    }
    if (!password || password.length < 6) {
      setError("Password must be at least 6 characters.");
      return;
    }

    startTransition(async () => {
      try {
        const gipResult = await signUpWithPassword(
          email.trim(),
          password,
          gipConfig,
        );

        const result = await customerSignUp({
          idToken: gipResult.idToken,
          uid: gipResult.uid,
          storeSlug,
        });

        if (!result.ok) {
          setError(result.message);
          return;
        }

        router.push(returnUrl);
        router.refresh();
      } catch (err) {
        if (err instanceof GIPError) {
          setError(err.message);
        } else {
          setError("Something went wrong. Please try again.");
        }
      }
    });
  }

  return (
    <form onSubmit={handleSubmit} noValidate className="mt-8 space-y-5">
      <div className="space-y-1.5">
        <label
          htmlFor="signup-email"
          className="block text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]"
        >
          Email address
        </label>
        <input
          id="signup-email"
          type="email"
          autoComplete="email"
          spellCheck={false}
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          className="w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-white px-3 py-2.5 text-base text-[color:var(--storefront-text,var(--ink-900))] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
        />
      </div>

      <div className="space-y-1.5">
        <label
          htmlFor="signup-password"
          className="block text-sm font-medium text-[color:var(--storefront-text,var(--ink-900))]"
        >
          Password
        </label>
        <input
          id="signup-password"
          type="password"
          autoComplete="new-password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          className="w-full rounded-md border border-[color:var(--storefront-text,var(--ink-900))]/20 bg-white px-3 py-2.5 text-base text-[color:var(--storefront-text,var(--ink-900))] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
        />
        <p className="text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-50">
          At least 6 characters.
        </p>
      </div>

      {error && (
        <p role="alert" className="text-sm text-[color:var(--signal)]">
          {error}
        </p>
      )}

      <button
        type="submit"
        disabled={pending}
        className="w-full rounded-md bg-[color:var(--storefront-accent,var(--ink-900))] px-6 py-3 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[color:var(--storefront-accent,var(--moss-700))]"
      >
        {pending ? "Creating account..." : "Create account"}
      </button>

      <p className="text-center text-xs text-[color:var(--storefront-text,var(--ink-900))] opacity-60">
        Already have an account?{" "}
        <Link
          href="/sign-in"
          className="text-[color:var(--storefront-accent,var(--moss-700))] underline underline-offset-4"
        >
          Sign in
        </Link>
      </p>
    </form>
  );
}
