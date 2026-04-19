/**
 * AppealForm — client component for the merchant-initiated arbitrage appeal.
 *
 * Endpoint: POST /api/v1/admin/stores/:storeId/arbitrage-appeal
 *
 * Fields:
 *   jurisdiction   — ISO-3166-1 alpha-2 via @tesserix/web Select
 *   justification  — textarea, max 2000 chars, live char counter
 *   document_url   — URL text input (v1 placeholder; TODO: wire GCS document
 *                    upload once a 'document' kind is added to brandingUpload.ts
 *                    and a corresponding upload-url route is created)
 *
 * Design: Paper · Ink · Moss. Editorial voice. H1 in Source Serif 4.
 * Hairline rules between field groups. Left-aligned 2/3 max-width column.
 */
'use client'

import { useRouter } from 'next/navigation'
import { useForm, Controller } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@tesserix/web'
import { useToast } from '@/components/feedback/Toaster'
import { useSubmitArbitrageAppeal } from '@/lib/api/subscription/hooks/useArbitrage'
import {
  arbitrageAppealRequestSchema,
  type ArbitrageAppealRequest,
} from '@/lib/api/subscription/schemas/arbitrage'
import { subscriptionCopy } from '@/lib/copy/subscription'
import { COUNTRY_OPTIONS } from '@/lib/data/countries'
import { ApiError } from '@/lib/api/client'

const copy = subscriptionCopy.arbitrage.appeal

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface AppealFormProps {
  storeId: string
}

// ---------------------------------------------------------------------------
// Field label + help text primitives
// ---------------------------------------------------------------------------

function FieldLabel({
  htmlFor,
  children,
}: {
  htmlFor: string
  children: React.ReactNode
}) {
  return (
    <label
      htmlFor={htmlFor}
      className="block text-sm font-medium text-[var(--ink-900)]"
      style={{ fontFamily: "'Source Sans 3', sans-serif" }}
    >
      {children}
    </label>
  )
}

function FieldHelp({ children }: { children: React.ReactNode }) {
  return (
    <p
      className="text-xs text-[var(--ink-700)]"
      style={{ fontFamily: "'Source Sans 3', sans-serif" }}
    >
      {children}
    </p>
  )
}

function FieldError({ message }: { message: string | undefined }) {
  if (!message) return null
  return (
    <p
      role="alert"
      className="text-xs text-[var(--danger)]"
      style={{ fontFamily: "'Source Sans 3', sans-serif" }}
    >
      {message}
    </p>
  )
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function AppealForm({ storeId }: AppealFormProps) {
  const router = useRouter()
  const { toast } = useToast()
  const submit = useSubmitArbitrageAppeal(storeId)

  const {
    register,
    handleSubmit,
    control,
    watch,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<ArbitrageAppealRequest>({
    resolver: zodResolver(arbitrageAppealRequestSchema),
    defaultValues: {
      jurisdiction: '',
      justification: '',
      document_url: undefined,
    },
  })

  const justificationValue = watch('justification') ?? ''
  const justificationLength = justificationValue.length

  function onSubmit(values: ArbitrageAppealRequest) {
    submit.mutate(values, {
      onSuccess: () => {
        toast.success(copy.toastSubmitted)
        router.push('/admin/settings/arbitrage')
      },
      onError: (err) => {
        if (err instanceof ApiError) {
          if (err.code === 'arbitrage_appeal_invalid_jurisdiction') {
            setError('jurisdiction', {
              message: 'This jurisdiction code is not recognised. Select a country from the list.',
            })
            return
          }
          if (err.code === 'arbitrage_appeal_no_open_flag') {
            setError('root', { message: copy.errorNoOpenFlag })
            return
          }
        }
        toast.error(copy.toastError)
      },
    })
  }

  return (
    <form
      onSubmit={(e) => {
        void handleSubmit(onSubmit)(e)
      }}
      className="space-y-0"
      noValidate
    >
      {/* Root-level form error (e.g. 404 no open flag) */}
      {errors.root?.message && (
        <div
          role="alert"
          className="mb-8 rounded-[6px] border border-[var(--hairline)] bg-[var(--paper-200)] px-4 py-3 text-sm text-[var(--ink-700)]"
          style={{ fontFamily: "'Source Sans 3', sans-serif" }}
        >
          {errors.root.message}
        </div>
      )}

      {/* ── Section: Jurisdiction ────────────────────────────────────────── */}
      <div className="border-b border-[var(--hairline)] pb-8">
        <div className="max-w-lg space-y-3">
          <div className="space-y-1">
            <FieldLabel htmlFor="jurisdiction">{copy.jurisdictionLabel}</FieldLabel>
            <FieldHelp>{copy.jurisdictionHelp}</FieldHelp>
          </div>

          <Controller
            name="jurisdiction"
            control={control}
            render={({ field }) => (
              <>
                {/* Hidden input so the Zod schema can coerce the value */}
                <input type="hidden" {...field} />
                <Select
                  value={field.value}
                  onValueChange={(val) => field.onChange(val)}
                >
                  <SelectTrigger
                    id="jurisdiction"
                    aria-required="true"
                    aria-invalid={Boolean(errors.jurisdiction)}
                    className="w-full"
                  >
                    <SelectValue placeholder="Select country" />
                  </SelectTrigger>
                  <SelectContent>
                    {COUNTRY_OPTIONS.map((opt) => (
                      <SelectItem key={opt.value} value={opt.value}>
                        {opt.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </>
            )}
          />

          <FieldError message={errors.jurisdiction?.message} />
        </div>
      </div>

      {/* ── Section: Justification ───────────────────────────────────────── */}
      <div className="border-b border-[var(--hairline)] py-8">
        <div className="max-w-lg space-y-3">
          <div className="space-y-1">
            <FieldLabel htmlFor="justification">{copy.justificationLabel}</FieldLabel>
            <FieldHelp>{copy.justificationHelp}</FieldHelp>
          </div>

          <textarea
            id="justification"
            rows={5}
            aria-invalid={Boolean(errors.justification)}
            {...register('justification')}
            className="w-full rounded-[6px] border border-[var(--hairline)] bg-[var(--background-elevated)] px-4 py-3 text-sm text-[var(--ink-900)] placeholder:text-[var(--ink-700)] focus:border-[var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[var(--moss-700)]"
            style={{ fontFamily: "'Source Sans 3', sans-serif" }}
            placeholder="Optional — describe anything unusual about your customer distribution."
          />

          <div className="flex items-center justify-between">
            <FieldError message={errors.justification?.message} />
            <span
              aria-live="polite"
              aria-label={`${justificationLength} of 2000 characters`}
              className="ml-auto text-xs text-[var(--ink-700)]"
              style={{ fontFamily: "'Source Sans 3', sans-serif" }}
            >
              {justificationLength}/2000
            </span>
          </div>
        </div>
      </div>

      {/* ── Section: Document URL ────────────────────────────────────────── */}
      {/* TODO(p16-task21): Replace with a GCS document upload widget once a
          'document' BrandingImageKind (or equivalent) is added to
          lib/brandingUpload.ts and a /branding/upload-url route accepts
          application/pdf + image/png for supporting documents. */}
      <div className="border-b border-[var(--hairline)] py-8">
        <div className="max-w-lg space-y-3">
          <div className="space-y-1">
            <FieldLabel htmlFor="document_url">{copy.uploadLabel}</FieldLabel>
            <FieldHelp>{copy.uploadHelp}</FieldHelp>
          </div>

          <input
            id="document_url"
            type="url"
            aria-invalid={Boolean(errors.document_url)}
            {...register('document_url')}
            className="h-11 w-full rounded-[6px] border border-[var(--hairline)] bg-[var(--background-elevated)] px-4 text-sm text-[var(--ink-900)] placeholder:text-[var(--ink-700)] focus:border-[var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[var(--moss-700)]"
            style={{ fontFamily: "'Source Sans 3', sans-serif" }}
            placeholder="https://storage.googleapis.com/…"
          />

          <FieldError message={errors.document_url?.message} />
        </div>
      </div>

      {/* ── Submit ───────────────────────────────────────────────────────── */}
      <div className="pt-8">
        <button
          type="submit"
          disabled={isSubmitting || submit.isPending}
          className="inline-flex h-11 items-center justify-center rounded-[6px] bg-[var(--ink-900)] px-6 text-sm font-medium text-[var(--background-elevated)] transition-colors hover:bg-[var(--ink-700)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50"
          style={{ fontFamily: "'Source Sans 3', sans-serif" }}
        >
          {submit.isPending ? 'Submitting\u2026' : copy.submitCta}
        </button>
      </div>
    </form>
  )
}
