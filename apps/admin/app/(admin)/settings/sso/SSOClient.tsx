"use client";

/**
 * Single sign-on configuration (#820).
 *
 * The screen the feature was missing: marketplace-api has had the config API
 * and the login flow for some time, and no merchant could reach either,
 * because there was nothing to click.
 *
 * Two behaviours here are deliberate and easy to get wrong:
 *
 *   1. The saved client-secret reference comes back REDACTED. Submitting the
 *      placeholder would store the literal string "[redacted]" and leave a
 *      configuration that looks right and cannot resolve a secret, so an
 *      untouched field is omitted rather than sent.
 *   2. "Test connection" reports what the server actually checked. A pass
 *      that only validated the shape says so, because a merchant who reads
 *      "connected" and then cannot sign in was told the wrong thing at the
 *      one moment they were paying attention.
 */

import { useEffect, useState } from "react";
import { Input, Label } from "@tesserix/web";

import { ApiError } from "@/lib/api/client";
import { ssoCopy } from "@/lib/copy/sso";
import { REDACTED, type SSOTestResult } from "@/lib/api/sso/schemas";
import {
  deleteSSOConfig,
  getSSOConfig,
  saveSSOConfig,
  testSSOConnection,
} from "@/lib/api/sso/sso";

type Status =
  | { kind: "idle" }
  | { kind: "busy"; label: string }
  | { kind: "ok"; message: string }
  | { kind: "error"; message: string };

interface FormState {
  issuer: string;
  clientId: string;
  secretRef: string;
  redirectUri: string;
  scopes: string;
  enabled: boolean;
}

const EMPTY: FormState = {
  issuer: "",
  clientId: "",
  secretRef: "",
  redirectUri: "",
  scopes: "",
  enabled: false,
};

export function SSOClient() {
  const [form, setForm] = useState<FormState>(EMPTY);
  const [hasSavedSecret, setHasSavedSecret] = useState(false);
  const [configured, setConfigured] = useState(false);
  const [loading, setLoading] = useState(true);
  const [planGated, setPlanGated] = useState(false);
  const [status, setStatus] = useState<Status>({ kind: "idle" });
  const [confirmingRemove, setConfirmingRemove] = useState(false);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const cfg = await getSSOConfig();
        if (cancelled) return;
        if (cfg) {
          const saved = cfg.metadata.client_secret_ref === REDACTED;
          setHasSavedSecret(saved);
          setConfigured(true);
          setForm({
            issuer: cfg.metadata.issuer,
            clientId: cfg.metadata.client_id,
            // Never prefill the placeholder into an editable field: it would
            // be submitted verbatim on the next save.
            secretRef: saved ? "" : cfg.metadata.client_secret_ref,
            redirectUri: cfg.metadata.redirect_uri,
            scopes: cfg.metadata.scopes,
            enabled: cfg.enabled,
          });
        }
      } catch (err) {
        if (cancelled) return;
        // 403 is the plan gate, not a failure — it deserves its own sentence
        // naming the plan rather than a generic error.
        if (err instanceof ApiError && err.status === 403) setPlanGated(true);
        else setStatus({ kind: "error", message: ssoCopy.loadError });
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  function set<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((f) => ({ ...f, [key]: value }));
    setStatus({ kind: "idle" });
  }

  async function handleSave(e: React.FormEvent) {
    e.preventDefault();
    setStatus({ kind: "busy", label: ssoCopy.saving });
    try {
      await saveSSOConfig({
        provider: "oidc",
        metadata: {
          issuer: form.issuer,
          client_id: form.clientId,
          client_secret_ref: form.secretRef,
          redirect_uri: form.redirectUri,
          scopes: form.scopes,
        },
        enabled: form.enabled,
      });
      setConfigured(true);
      if (form.secretRef.trim()) setHasSavedSecret(true);
      setStatus({ kind: "ok", message: ssoCopy.saved });
    } catch {
      setStatus({ kind: "error", message: ssoCopy.saveError });
    }
  }

  async function handleTest() {
    setStatus({ kind: "busy", label: ssoCopy.testing });
    try {
      const result: SSOTestResult = await testSSOConnection();
      if (!result.ok) {
        setStatus({
          kind: "error",
          message: ssoCopy.testFailed(result.error ?? ""),
        });
        return;
      }
      setStatus({
        kind: "ok",
        message:
          result.checked === "idp"
            ? ssoCopy.testReachedIdP
            : ssoCopy.testConfigOnly,
      });
    } catch {
      setStatus({ kind: "error", message: ssoCopy.testError });
    }
  }

  async function handleRemove() {
    setConfirmingRemove(false);
    setStatus({ kind: "busy", label: ssoCopy.removing });
    try {
      await deleteSSOConfig();
      setForm(EMPTY);
      setHasSavedSecret(false);
      setConfigured(false);
      setStatus({ kind: "ok", message: ssoCopy.removed });
    } catch {
      setStatus({ kind: "error", message: ssoCopy.saveError });
    }
  }

  if (loading) {
    return <p className="text-sm text-foreground-tertiary">Loading…</p>;
  }

  if (planGated) {
    return (
      <p className="max-w-xl text-sm leading-6 text-[var(--ink-700)]">
        {ssoCopy.planGated}
      </p>
    );
  }

  const busy = status.kind === "busy";

  return (
    <form onSubmit={handleSave} className="max-w-xl space-y-6" noValidate>
      <Field
        id="sso-issuer"
        label={ssoCopy.fields.issuerLabel}
        hint={ssoCopy.fields.issuerHint}
      >
        <Input
          id="sso-issuer"
          type="url"
          inputMode="url"
          autoComplete="off"
          spellCheck={false}
          required
          value={form.issuer}
          onChange={(e) => set("issuer", e.target.value)}
          placeholder="https://accounts.example.com"
        />
      </Field>

      <Field
        id="sso-client-id"
        label={ssoCopy.fields.clientIdLabel}
        hint={ssoCopy.fields.clientIdHint}
      >
        <Input
          id="sso-client-id"
          autoComplete="off"
          spellCheck={false}
          required
          value={form.clientId}
          onChange={(e) => set("clientId", e.target.value)}
        />
      </Field>

      <Field
        id="sso-secret-ref"
        label={ssoCopy.fields.secretRefLabel}
        hint={
          hasSavedSecret
            ? ssoCopy.fields.secretRefKept
            : ssoCopy.fields.secretRefHint
        }
      >
        <Input
          id="sso-secret-ref"
          autoComplete="off"
          spellCheck={false}
          required={!hasSavedSecret}
          value={form.secretRef}
          onChange={(e) => set("secretRef", e.target.value)}
          placeholder={hasSavedSecret ? "••••••••" : "kv/your-org/idp"}
        />
      </Field>

      <Field
        id="sso-redirect"
        label={ssoCopy.fields.redirectLabel}
        hint={ssoCopy.fields.redirectHint}
      >
        <Input
          id="sso-redirect"
          type="url"
          inputMode="url"
          autoComplete="off"
          spellCheck={false}
          value={form.redirectUri}
          onChange={(e) => set("redirectUri", e.target.value)}
        />
      </Field>

      <Field
        id="sso-scopes"
        label={ssoCopy.fields.scopesLabel}
        hint={ssoCopy.fields.scopesHint}
      >
        <Input
          id="sso-scopes"
          autoComplete="off"
          spellCheck={false}
          value={form.scopes}
          onChange={(e) => set("scopes", e.target.value)}
          placeholder="openid email profile"
        />
      </Field>

      <div className="border-t border-[var(--hairline,var(--ink-100))] pt-6">
        <label className="flex cursor-pointer items-start gap-3">
          <input
            type="checkbox"
            className="mt-0.5 h-4 w-4 shrink-0 accent-[var(--moss-700)]"
            checked={form.enabled}
            onChange={(e) => set("enabled", e.target.checked)}
          />
          <span>
            <span className="block text-sm text-foreground">
              {ssoCopy.fields.enabledLabel}
            </span>
            <span className="mt-1 block text-xs text-foreground-tertiary">
              {ssoCopy.fields.enabledHint}
            </span>
          </span>
        </label>
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <button
          type="submit"
          disabled={busy}
          aria-busy={busy}
          className="inline-flex h-10 items-center rounded-md bg-[var(--ink-900)] px-5 text-sm font-medium text-white transition-colors hover:bg-[var(--ink-900)]/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-60"
        >
          {status.kind === "busy" && status.label === ssoCopy.saving
            ? ssoCopy.saving
            : ssoCopy.save}
        </button>

        {configured && (
          <button
            type="button"
            onClick={handleTest}
            disabled={busy}
            className="inline-flex h-10 items-center rounded-md border border-[var(--ink-900)] px-5 text-sm font-medium text-[var(--ink-900)] transition-colors hover:bg-[var(--paper-200)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] disabled:cursor-not-allowed disabled:opacity-60"
          >
            {status.kind === "busy" && status.label === ssoCopy.testing
              ? ssoCopy.testing
              : ssoCopy.test}
          </button>
        )}

        {configured && (
          <button
            type="button"
            onClick={() => setConfirmingRemove(true)}
            disabled={busy}
            className="text-sm text-[var(--danger,#7a1a1a)] underline underline-offset-4 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] disabled:opacity-60"
          >
            {ssoCopy.remove}
          </button>
        )}
      </div>

      <div className="min-h-[1.25rem]" aria-live="polite">
        {status.kind === "ok" && (
          <p className="text-sm text-[var(--moss-700)]">{status.message}</p>
        )}
        {status.kind === "error" && (
          <p role="alert" className="text-sm text-[var(--danger,#7a1a1a)]">
            {status.message}
          </p>
        )}
      </div>

      <p className="text-xs text-foreground-tertiary">{ssoCopy.samlNote}</p>

      {confirmingRemove && (
        <div
          role="alertdialog"
          aria-labelledby="sso-remove-title"
          aria-describedby="sso-remove-body"
          className="rounded-md border border-[var(--hairline,var(--ink-100))] bg-[var(--background-elevated,#fff)] p-5"
        >
          <h3
            id="sso-remove-title"
            className="font-serif text-lg text-[var(--ink-900)]"
          >
            {ssoCopy.removeConfirmTitle}
          </h3>
          <p id="sso-remove-body" className="mt-2 text-sm leading-6 text-[var(--ink-700)]">
            {ssoCopy.removeConfirmBody}
          </p>
          <div className="mt-4 flex gap-3">
            <button
              type="button"
              onClick={handleRemove}
              className="inline-flex h-9 items-center rounded-md bg-[var(--danger,#7a1a1a)] px-4 text-sm font-medium text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]"
            >
              {ssoCopy.removeConfirmCta}
            </button>
            <button
              type="button"
              onClick={() => setConfirmingRemove(false)}
              className="inline-flex h-9 items-center rounded-md px-4 text-sm text-[var(--ink-700)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]"
            >
              {ssoCopy.cancel}
            </button>
          </div>
        </div>
      )}
    </form>
  );
}

/** Label + control + a reserved hint row, matching the onboarding form's shape. */
function Field({
  id,
  label,
  hint,
  children,
}: {
  id: string;
  label: string;
  hint: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id} className="text-foreground">
        {label}
      </Label>
      {children}
      <p id={`${id}-hint`} className="text-xs leading-5 text-foreground-tertiary">
        {hint}
      </p>
    </div>
  );
}
