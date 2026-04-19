/**
 * TaxIdForm — client component for tax-ID submission and re-submission.
 *
 * Endpoint: POST /api/v1/admin/stores/:storeId/tax/submit
 *
 * Fields:
 *   business_name    — text input, required
 *   country          — select (COUNTRY_OPTIONS), required, 2-char
 *   tax_id           — text input, required
 *   billing_address  — text input, optional
 *
 * Status states:
 *   not_submitted  — form is editable, submit CTA shown
 *   pending        — form shown, note that review is in progress
 *   validated      — read-only success state; Update CTA reveals form
 *   clock_paused   — ClockPauseBadge shown; form shown for re-submission
 *   rejected       — error state with code; form shown for re-submission
 *
 * Design: Paper · Ink · Moss. Editorial voice. Left-aligned max-w-2xl.
 * Hairline rules between field groups. Source Serif 4 heading, Source Sans 3 body.
 */
'use client'

import { useState } from 'react'
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
import { useTaxIdStatus, useSubmitTaxId } from '@/lib/api/subscription/hooks/useTaxId'
import {
  taxIdSubmitRequestSchema,
  type TaxIdSubmitRequest,
} from '@/lib/api/subscription/schemas/taxId'
import { subscriptionCopy } from '@/lib/copy/subscription'
import { COUNTRY_OPTIONS } from '@/lib/data/countries'
import { ApiError } from '@/lib/api/client'
import { ClockPauseBadge } from './ClockPauseBadge'

const copy = subscriptionCopy.taxId

// ---------------------------------------------------------------------------
// Field primitives
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
// Status panel — shown when validated or clock_paused; no form rendered
// ---------------------------------------------------------------------------

function StatusPanel({ storeId }: { storeId: string; onUpdate: () => void }) {
  const { status, pauseReason, validationCode } = useTaxIdStatus(storeId)
  const statusCopy = copy.status

  if (status === 'validated') {
    return (
      <div className="space-y-1">
        <p
          className="text-sm font-medium text-[var(--ink-900)]"
          style={{ fontFamily: "'Source Sans 3', sans-serif" }}
        >
          {statusCopy.validatedHeading}
        </p>
        <p
          className="text-sm text-[var(--ink-700)]"
          style={{ fontFamily: "'Source Sans 3', sans-serif" }}
        >
          {statusCopy.validatedBody}
        </p>
      </div>
    )
  }

  if (status === 'rejected' && validationCode) {
    return (
      <div className="space-y-1">
        <p
          className="text-sm font-medium text-[var(--danger)]"
          style={{ fontFamily: "'Source Sans 3', sans-serif" }}
        >
          {statusCopy.rejectedHeading}
        </p>
        <p
          className="text-sm text-[var(--ink-700)]"
          style={{ fontFamily: "'Source Sans 3', sans-serif" }}
        >
          {statusCopy.rejectedBodyTemplate(validationCode)}
        </p>
      </div>
    )
  }

  return null
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface TaxIdFormProps {
  storeId: string
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function TaxIdForm({ storeId }: TaxIdFormProps) {
  const { toast } = useToast()
  const taxIdState = useTaxIdStatus(storeId)
  const submitMutation = useSubmitTaxId(storeId)

  // When status is 'validated', the form is hidden by default. The "Update"
  // CTA toggles it visible.
  const [showForm, setShowForm] = useState(
    taxIdState.status !== 'validated',
  )

  const {
    register,
    handleSubmit,
    control,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<TaxIdSubmitRequest>({
    resolver: zodResolver(taxIdSubmitRequestSchema),
    defaultValues: {
      business_name: '',
      country: '',
      tax_id: '',
      billing_address: '',
    },
  })

  function onSubmit(values: TaxIdSubmitRequest) {
    submitMutation.mutate(values, {
      onSuccess: () => {
        toast.success(copy.toastSubmitted)
        setShowForm(false)
      },
      onError: (err) => {
        if (err instanceof ApiError) {
          if (err.code === 'invalid_format') {
            setError('tax_id', {
              message:
                'The format of your tax ID looks incorrect. Check it matches your country\u2019s format and try again.',
            })
            return
          }
          if (err.code === 'tax_id_not_found') {
            setError('tax_id', {
              message:
                'We couldn\u2019t find this tax ID in the registry. Verify the number is correct.',
            })
            return
          }
          if (err.code === 'validator_disabled') {
            setError('country', {
              message:
                'Validation for this country is not yet available. Contact support if this is blocking your onboarding.',
            })
            return
          }
        }
        toast.error(copy.toastError)
      },
    })
  }

  const { status, pauseReason } = taxIdState

  return (
    <div className="space-y-0">
      {/* ── Status banner for validated state ─────────────────────────── */}
      {status === 'validated' && (
        <div className="mb-8 space-y-3">
          <StatusPanel storeId={storeId} onUpdate={() => setShowForm(true)} />
          {!showForm && (
            <button
              type="button"
              onClick={() => setShowForm(true)}
              className="inline-flex h-9 items-center rounded-[6px] border border-[var(--hairline)] bg-[var(--background-elevated)] px-4 text-sm font-medium text-[var(--ink-900)] transition-colors hover:bg-[var(--paper-200)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] focus-visible:ring-offset-2"
              style={{ fontFamily: "'Source Sans 3', sans-serif" }}
            >
              {copy.updateCta}
            </button>
          )}
        </div>
      )}

      {/* ── Clock-pause badge + status copy ───────────────────────────── */}
      {status === 'clock_paused' && pauseReason && (
        <div className="mb-8 space-y-2">
          <ClockPauseBadge reason={pauseReason} />
          <p
            className="text-sm text-[var(--ink-700)]"
            style={{ fontFamily: "'Source Sans 3', sans-serif" }}
          >
            {pauseReason === 'registry_unavailable'
              ? copy.status.pausedRegistryBody
              : copy.status.pausedSeaBody}
          </p>
        </div>
      )}

      {/* ── Rejected status copy ───────────────────────────────────────── */}
      {status === 'rejected' && taxIdState.validationCode && (
        <div className="mb-8">
          <StatusPanel storeId={storeId} onUpdate={() => undefined} />
        </div>
      )}

      {/* ── Form ─────────────────────────────────────────────────────────── */}
      {showForm && (
        <form
          onSubmit={(e) => {
            void handleSubmit(onSubmit)(e)
          }}
          className="space-y-0"
          noValidate
        >
          {/* Root-level error */}
          {errors.root?.message && (
            <div
              role="alert"
              className="mb-8 rounded-[6px] border border-[var(--hairline)] bg-[var(--paper-200)] px-4 py-3 text-sm text-[var(--ink-700)]"
              style={{ fontFamily: "'Source Sans 3', sans-serif" }}
            >
              {errors.root.message}
            </div>
          )}

          {/* ── Business name ────────────────────────────────────────────── */}
          <div className="border-b border-[var(--hairline)] pb-8">
            <div className="max-w-lg space-y-3">
              <div className="space-y-1">
                <FieldLabel htmlFor="business_name">
                  {copy.businessNameLabel}
                </FieldLabel>
              </div>
              <input
                id="business_name"
                type="text"
                aria-required="true"
                aria-invalid={Boolean(errors.business_name)}
                {...register('business_name')}
                className="h-11 w-full rounded-[6px] border border-[var(--hairline)] bg-[var(--background-elevated)] px-4 text-sm text-[var(--ink-900)] placeholder:text-[var(--ink-700)] focus:border-[var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[var(--moss-700)]"
                style={{ fontFamily: "'Source Sans 3', sans-serif" }}
                placeholder="Acme Ltd"
              />
              <FieldError message={errors.business_name?.message} />
            </div>
          </div>

          {/* ── Country of registration ──────────────────────────────────── */}
          <div className="border-b border-[var(--hairline)] py-8">
            <div className="max-w-lg space-y-3">
              <div className="space-y-1">
                <FieldLabel htmlFor="country">{copy.countryLabel}</FieldLabel>
              </div>

              <Controller
                name="country"
                control={control}
                render={({ field }) => (
                  <>
                    <input type="hidden" {...field} />
                    <Select
                      value={field.value}
                      onValueChange={(val) => field.onChange(val)}
                    >
                      <SelectTrigger
                        id="country"
                        aria-required="true"
                        aria-invalid={Boolean(errors.country)}
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

              <FieldError message={errors.country?.message} />
            </div>
          </div>

          {/* ── Tax ID ───────────────────────────────────────────────────── */}
          <div className="border-b border-[var(--hairline)] py-8">
            <div className="max-w-lg space-y-3">
              <div className="space-y-1">
                <FieldLabel htmlFor="tax_id">{copy.taxIdLabel}</FieldLabel>
                <FieldHelp>{copy.taxIdHelp}</FieldHelp>
              </div>
              <input
                id="tax_id"
                type="text"
                aria-required="true"
                aria-invalid={Boolean(errors.tax_id)}
                {...register('tax_id')}
                className="h-11 w-full rounded-[6px] border border-[var(--hairline)] bg-[var(--background-elevated)] px-4 text-sm text-[var(--ink-900)] placeholder:text-[var(--ink-700)] focus:border-[var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[var(--moss-700)]"
                style={{ fontFamily: "'Source Sans 3', sans-serif" }}
                placeholder="e.g. GB123456789"
              />
              <FieldError message={errors.tax_id?.message} />
            </div>
          </div>

          {/* ── Billing address (optional) ───────────────────────────────── */}
          <div className="border-b border-[var(--hairline)] py-8">
            <div className="max-w-lg space-y-3">
              <div className="space-y-1">
                <FieldLabel htmlFor="billing_address">
                  {copy.billingAddressLabel}
                </FieldLabel>
                <FieldHelp>{copy.billingAddressHelp}</FieldHelp>
              </div>
              <input
                id="billing_address"
                type="text"
                {...register('billing_address')}
                className="h-11 w-full rounded-[6px] border border-[var(--hairline)] bg-[var(--background-elevated)] px-4 text-sm text-[var(--ink-900)] placeholder:text-[var(--ink-700)] focus:border-[var(--moss-700)] focus:outline-none focus:ring-1 focus:ring-[var(--moss-700)]"
                style={{ fontFamily: "'Source Sans 3', sans-serif" }}
                placeholder="123 High Street, London, EC1A 1BB"
              />
            </div>
          </div>

          {/* ── Submit ────────────────────────────────────────────────────── */}
          <div className="flex items-center gap-4 pt-8">
            <button
              type="submit"
              disabled={isSubmitting || submitMutation.isPending}
              className="inline-flex h-11 items-center justify-center rounded-[6px] bg-[var(--ink-900)] px-6 text-sm font-medium text-[var(--background-elevated)] transition-colors hover:bg-[var(--ink-700)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50"
              style={{ fontFamily: "'Source Sans 3', sans-serif" }}
            >
              {submitMutation.isPending
                ? 'Submitting\u2026'
                : status === 'validated'
                  ? copy.updateCta
                  : copy.submitCta}
            </button>

            {/* Cancel update — only shown when validated + form revealed */}
            {status === 'validated' && (
              <button
                type="button"
                onClick={() => setShowForm(false)}
                className="inline-flex h-11 items-center justify-center rounded-[6px] border border-[var(--hairline)] bg-transparent px-6 text-sm font-medium text-[var(--ink-900)] transition-colors hover:bg-[var(--paper-200)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] focus-visible:ring-offset-2"
                style={{ fontFamily: "'Source Sans 3', sans-serif" }}
              >
                Cancel
              </button>
            )}
          </div>
        </form>
      )}
    </div>
  )
}
