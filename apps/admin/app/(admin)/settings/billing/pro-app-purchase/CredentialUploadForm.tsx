'use client'

/**
 * CredentialUploadForm — Apple App Store Connect and Google Play credential
 * upload for the White-label App add-on.
 *
 * Layout:
 *   - Intro paragraph stating credentials are encrypted at rest.
 *   - Two panels side-by-side on desktop (Apple | Google), stacked on mobile.
 *   - Hairline rule between them on desktop; full-width on mobile.
 *   - Support link at the bottom.
 *
 * Security:
 *   - File contents are read once on file-select and stored in RHF state
 *     only for the lifetime of the form. The field is cleared in the form's
 *     reset on unmount. Raw file objects are never kept in state.
 *   - Credentials are sent as multipart/form-data blobs and are not stored
 *     in localStorage, sessionStorage, or any persistent browser store.
 *
 * Backend notes:
 *   - Apple: multipart fields `p8` (file), `issuer_id`, `key_id`. Returns 204.
 *     NO bundle_id field in the Go handler.
 *   - Google: multipart field `service_account_json` (file). Returns 204.
 *   - Both gates require Pro plan + white_label_app add-on active (403 add_on_not_active).
 *
 * Accessibility:
 *   - Every field has a visible <label> associated via htmlFor/id.
 *   - aria-invalid + aria-describedby for validation errors.
 *   - File inputs show a custom label trigger; the actual <input type="file"> is
 *     visually hidden but keyboard accessible.
 *   - Error messages announced via role="alert".
 */

import { useId, useRef, useEffect } from 'react'
import { useForm, type SubmitHandler } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import Link from 'next/link'
import {
  appleCredentialsFormSchema,
  googleCredentialsFormSchema,
  type AppleCredentialsForm,
  type GoogleCredentialsForm,
} from '@/lib/api/subscription/schemas/appCredentials'
import {
  useUploadAppleCredentials,
  useUploadGoogleCredentials,
} from '@/lib/api/subscription/hooks/useAppCredentials'
import { useToast } from '@/components/feedback/Toaster'
import { subscriptionCopy } from '@/lib/copy/subscription'

// ---------------------------------------------------------------------------
// Types + constants
// ---------------------------------------------------------------------------

interface CredentialUploadFormProps {
  storeId: string
}

const copy = subscriptionCopy.appCredentials

// ---------------------------------------------------------------------------
// Shared Field wrapper
// ---------------------------------------------------------------------------

interface FieldProps {
  id: string
  label: string
  helpText?: string
  error?: string
  children: React.ReactNode
}

function Field({ id, label, helpText, error, children }: FieldProps) {
  const errorId = `${id}-error`
  const helpId = `${id}-help`
  return (
    <div className="flex flex-col gap-1.5">
      <label
        htmlFor={id}
        className="text-sm font-medium text-[var(--ink-900)]"
      >
        {label}
      </label>
      {children}
      {helpText && !error && (
        <span
          id={helpId}
          className="text-xs leading-5 text-[var(--ink-500)]"
        >
          {helpText}
        </span>
      )}
      {error && (
        <span
          id={errorId}
          role="alert"
          className="text-xs text-[var(--danger)]"
        >
          {error}
        </span>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Input class helper
// ---------------------------------------------------------------------------

function inputClass(hasError?: boolean): string {
  return [
    'h-9 w-full rounded-md border bg-[var(--background-elevated)] px-3 text-sm text-[var(--ink-900)]',
    'placeholder:text-[var(--ink-400)] focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)] focus:ring-offset-1',
    hasError ? 'border-[var(--danger)]' : 'border-[var(--hairline)]',
  ].join(' ')
}

// ---------------------------------------------------------------------------
// File-read helper — returns text content or null
// ---------------------------------------------------------------------------

function readFileAsText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = (e) => resolve((e.target?.result as string) ?? '')
    reader.onerror = () => reject(new Error('Could not read file'))
    reader.readAsText(file)
  })
}

// ---------------------------------------------------------------------------
// ApplePanel
// ---------------------------------------------------------------------------

interface ApplePanelProps {
  storeId: string
}

function ApplePanel({ storeId }: ApplePanelProps) {
  const { toast } = useToast()
  const mutation = useUploadAppleCredentials(storeId)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const formId = useId()
  const ids = {
    keyId: `${formId}-key-id`,
    issuerId: `${formId}-issuer-id`,
    p8File: `${formId}-p8-file`,
  }

  const {
    register,
    handleSubmit,
    setValue,
    reset,
    formState: { errors, isSubmitting },
  } = useForm<AppleCredentialsForm>({
    resolver: zodResolver(appleCredentialsFormSchema),
    defaultValues: {
      key_id: '',
      issuer_id: '',
      p8_contents: '',
    },
  })

  // Clear sensitive form state on unmount
  useEffect(() => {
    return () => {
      reset()
    }
  }, [reset])

  const isPending = isSubmitting || mutation.isPending

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    try {
      const text = await readFileAsText(file)
      setValue('p8_contents', text, { shouldValidate: true })
    } catch {
      setValue('p8_contents', '', { shouldValidate: true })
    }
    // Clear the file input reference so replacing works correctly
    if (fileInputRef.current) {
      fileInputRef.current.value = ''
    }
  }

  const onSubmit: SubmitHandler<AppleCredentialsForm> = (values) => {
    mutation.mutate(values, {
      onSuccess: () => {
        toast.success(copy.toastUploaded)
        reset()
      },
      onError: () => {
        toast.error(copy.toastError)
      },
    })
  }

  return (
    <section aria-labelledby={`${formId}-apple-heading`} className="flex-1 min-w-0">
      <h2
        id={`${formId}-apple-heading`}
        className="font-serif text-xl font-medium tracking-tight text-[var(--ink-900)]"
      >
        {copy.apple.heading}
      </h2>
      <p className="mt-2 text-sm leading-6 text-[var(--ink-700)]">
        {copy.apple.help}
      </p>

      <form
        onSubmit={handleSubmit(onSubmit)}
        noValidate
        aria-label={copy.apple.heading}
        className="mt-5 space-y-4"
      >
        <Field
          id={ids.keyId}
          label={copy.apple.keyIdLabel}
          error={errors.key_id?.message}
        >
          <input
            id={ids.keyId}
            type="text"
            autoComplete="off"
            aria-invalid={errors.key_id ? true : undefined}
            aria-describedby={errors.key_id ? `${ids.keyId}-error` : undefined}
            className={inputClass(Boolean(errors.key_id))}
            {...register('key_id')}
          />
        </Field>

        <Field
          id={ids.issuerId}
          label={copy.apple.issuerIdLabel}
          error={errors.issuer_id?.message}
        >
          <input
            id={ids.issuerId}
            type="text"
            autoComplete="off"
            aria-invalid={errors.issuer_id ? true : undefined}
            aria-describedby={
              errors.issuer_id ? `${ids.issuerId}-error` : undefined
            }
            className={inputClass(Boolean(errors.issuer_id))}
            {...register('issuer_id')}
          />
        </Field>

        <Field
          id={ids.p8File}
          label={copy.apple.p8FileLabel}
          error={errors.p8_contents?.message}
          helpText="The .p8 file downloaded from App Store Connect."
        >
          {/* Hidden native file input — triggered by the label button below */}
          <input
            ref={fileInputRef}
            id={ids.p8File}
            type="file"
            accept=".p8"
            aria-invalid={errors.p8_contents ? true : undefined}
            aria-describedby={
              errors.p8_contents
                ? `${ids.p8File}-error`
                : `${ids.p8File}-help`
            }
            className="sr-only"
            onChange={handleFileChange}
          />
          <label
            htmlFor={ids.p8File}
            className={[
              'inline-flex h-9 cursor-pointer items-center rounded-md border px-3 text-sm',
              'transition-colors hover:bg-[var(--paper-200)] focus-within:ring-2 focus-within:ring-[var(--moss-700)] focus-within:ring-offset-1',
              errors.p8_contents
                ? 'border-[var(--danger)] text-[var(--danger)]'
                : 'border-[var(--hairline)] text-[var(--ink-700)]',
            ].join(' ')}
          >
            Choose .p8 file
          </label>
        </Field>

        <div className="pt-2">
          <button
            type="submit"
            disabled={isPending}
            className="inline-flex h-9 items-center justify-center rounded-md bg-[var(--ink-900)] px-5 text-sm font-medium text-[var(--paper-200)] transition-colors hover:bg-[var(--moss-700)] focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)] focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {isPending ? 'Saving\u2026' : copy.apple.submitCta}
          </button>
        </div>
      </form>
    </section>
  )
}

// ---------------------------------------------------------------------------
// GooglePanel
// ---------------------------------------------------------------------------

interface GooglePanelProps {
  storeId: string
}

function GooglePanel({ storeId }: GooglePanelProps) {
  const { toast } = useToast()
  const mutation = useUploadGoogleCredentials(storeId)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const formId = useId()
  const serviceAccountId = `${formId}-service-account`

  const {
    setValue,
    handleSubmit,
    reset,
    formState: { errors, isSubmitting },
  } = useForm<GoogleCredentialsForm>({
    resolver: zodResolver(googleCredentialsFormSchema),
    defaultValues: {
      service_account_json: '',
    },
  })

  // Clear sensitive form state on unmount
  useEffect(() => {
    return () => {
      reset()
    }
  }, [reset])

  const isPending = isSubmitting || mutation.isPending

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    try {
      const text = await readFileAsText(file)
      setValue('service_account_json', text, { shouldValidate: true })
    } catch {
      setValue('service_account_json', '', { shouldValidate: true })
    }
    if (fileInputRef.current) {
      fileInputRef.current.value = ''
    }
  }

  const onSubmit: SubmitHandler<GoogleCredentialsForm> = (values) => {
    mutation.mutate(values, {
      onSuccess: () => {
        toast.success(copy.toastUploaded)
        reset()
      },
      onError: () => {
        toast.error(copy.toastError)
      },
    })
  }

  return (
    <section aria-labelledby={`${formId}-google-heading`} className="flex-1 min-w-0">
      <h2
        id={`${formId}-google-heading`}
        className="font-serif text-xl font-medium tracking-tight text-[var(--ink-900)]"
      >
        {copy.google.heading}
      </h2>
      <p className="mt-2 text-sm leading-6 text-[var(--ink-700)]">
        {copy.google.help}
      </p>

      <form
        onSubmit={handleSubmit(onSubmit)}
        noValidate
        aria-label={copy.google.heading}
        className="mt-5 space-y-4"
      >
        <Field
          id={serviceAccountId}
          label={copy.google.serviceAccountLabel}
          error={
            errors.service_account_json
              ? copy.google.invalidJsonError
              : undefined
          }
          helpText="A Google Cloud service account JSON with Play Console access."
        >
          {/* Hidden native file input */}
          <input
            ref={fileInputRef}
            id={serviceAccountId}
            type="file"
            accept=".json,application/json"
            aria-invalid={errors.service_account_json ? true : undefined}
            aria-describedby={
              errors.service_account_json
                ? `${serviceAccountId}-error`
                : `${serviceAccountId}-help`
            }
            className="sr-only"
            onChange={handleFileChange}
          />
          <label
            htmlFor={serviceAccountId}
            className={[
              'inline-flex h-9 cursor-pointer items-center rounded-md border px-3 text-sm',
              'transition-colors hover:bg-[var(--paper-200)] focus-within:ring-2 focus-within:ring-[var(--moss-700)] focus-within:ring-offset-1',
              errors.service_account_json
                ? 'border-[var(--danger)] text-[var(--danger)]'
                : 'border-[var(--hairline)] text-[var(--ink-700)]',
            ].join(' ')}
          >
            Choose JSON file
          </label>
        </Field>

        <div className="pt-2">
          <button
            type="submit"
            disabled={isPending}
            className="inline-flex h-9 items-center justify-center rounded-md bg-[var(--ink-900)] px-5 text-sm font-medium text-[var(--paper-200)] transition-colors hover:bg-[var(--moss-700)] focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)] focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {isPending ? 'Saving\u2026' : copy.google.submitCta}
          </button>
        </div>
      </form>
    </section>
  )
}

// ---------------------------------------------------------------------------
// CredentialUploadForm
// ---------------------------------------------------------------------------

export function CredentialUploadForm({ storeId }: CredentialUploadFormProps) {
  return (
    <div className="max-w-4xl space-y-8">
      {/* Two-panel layout: side-by-side on md+, stacked below */}
      <div className="flex flex-col gap-10 md:flex-row md:gap-0 md:divide-x md:divide-[var(--hairline)]">
        <div className="md:pr-10 md:flex-1">
          <ApplePanel storeId={storeId} />
        </div>
        <div className="md:pl-10 md:flex-1">
          <GooglePanel storeId={storeId} />
        </div>
      </div>

      {/* Hairline + support link */}
      <div className="border-t border-[var(--hairline)] pt-6">
        <p className="text-sm text-[var(--ink-700)]">
          {copy.supportHint}{' '}
          <Link
            href="/settings/support"
            className="text-[var(--moss-700)] underline-offset-2 hover:underline focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)] focus:ring-offset-1 rounded"
          >
            {copy.supportCta}
          </Link>
        </p>
      </div>
    </div>
  )
}
