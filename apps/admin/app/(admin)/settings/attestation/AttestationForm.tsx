/**
 * AttestationForm — client component for the US/CA business-entity attestation
 * required by §19.3.1.
 *
 * Endpoint: POST /api/v1/admin/stores/:storeId/tax/attestation
 *
 * Fields (form layer):
 *   country          — Select, restricted to US | CA
 *   is_business_entity — required checkbox; Submit CTA disabled until ticked
 *
 * Wire layer (sent to backend):
 *   country          — from form select
 *   checkbox_text    — verbatim copy.attestation.checkboxLabel (v1 text)
 *   checkbox_version — copy.attestation.checkboxVersion ('v1')
 *
 * Once signed, the page shows the "Attestation on file" read-only state.
 * The attestation row is append-only on the server (UPDATE-blocking trigger).
 *
 * Design: Paper · Ink · Moss. Editorial voice. Left-aligned max-w-lg.
 * Hairline rules between sections. Source Serif 4 heading, Source Sans 3 body.
 */
'use client'

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
import {
  useAttestation,
  useSignAttestation,
} from '@/lib/api/subscription/hooks/useAttestation'
import {
  attestationFormSchema,
  type AttestationFormValues,
  type AttestationRequest,
} from '@/lib/api/subscription/schemas/attestation'
import { subscriptionCopy } from '@/lib/copy/subscription'
import { formatBillingDate } from '@/lib/format/date'

const copy = subscriptionCopy.attestation

// ---------------------------------------------------------------------------
// Jurisdiction options — US and CA only (§19.3.1)
// ---------------------------------------------------------------------------

const JURISDICTION_OPTIONS = [
  { value: 'US', label: 'United States (US)' },
  { value: 'CA', label: 'Canada (CA)' },
] as const

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
// Signed state — read-only confirmation panel
// ---------------------------------------------------------------------------

function SignedPanel({ acceptedAt }: { acceptedAt: string | null }) {
  const formattedDate = acceptedAt ? formatBillingDate(acceptedAt) : ''

  return (
    <div className="space-y-3">
      <div className="space-y-1">
        <p
          className="text-sm font-medium text-[var(--ink-900)]"
          style={{ fontFamily: "'Source Sans 3', sans-serif" }}
        >
          {copy.signedHeading}
        </p>
        <p
          className="text-sm text-[var(--ink-700)]"
          style={{ fontFamily: "'Source Sans 3', sans-serif" }}
        >
          {formattedDate
            ? copy.signedBodyTemplate(formattedDate)
            : copy.supportHint}
        </p>
      </div>
      {/* Only show the trailing support hint when we also have a date — otherwise
          the body line already shows the hint and we avoid duplicate text. */}
      {formattedDate && (
        <p
          className="text-xs text-[var(--ink-700)]"
          style={{ fontFamily: "'Source Sans 3', sans-serif" }}
        >
          {copy.supportHint}
        </p>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface AttestationFormProps {
  storeId: string
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function AttestationForm({ storeId }: AttestationFormProps) {
  const { toast } = useToast()
  const attestationState = useAttestation(storeId)
  const signMutation = useSignAttestation(storeId)

  const {
    handleSubmit,
    control,
    watch,
    formState: { errors, isSubmitting },
  } = useForm<AttestationFormValues>({
    resolver: zodResolver(attestationFormSchema),
    defaultValues: {
      country: undefined,
      is_business_entity: undefined,
    },
  })

  // Submit CTA disabled until the checkbox is ticked — derived from form state.
  const isBusinessEntityChecked = watch('is_business_entity')

  // If already signed, show read-only panel immediately.
  if (attestationState.isSigned) {
    return <SignedPanel acceptedAt={attestationState.acceptedAt} />
  }

  function onSubmit(values: AttestationFormValues) {
    // Build the wire body from form values + copy constants.
    // checkbox_text must be the exact label shown to the user; the backend
    // stores it immutably for audit trail purposes (§18.8).
    const wireBody: AttestationRequest = {
      country: values.country,
      checkbox_text: copy.checkboxLabel,
      checkbox_version: copy.checkboxVersion,
    }

    signMutation.mutate(wireBody, {
      onSuccess: () => {
        toast.success(copy.toastSigned)
      },
      onError: () => {
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
      {/* ── Jurisdiction select ──────────────────────────────────────────── */}
      <div className="border-b border-[var(--hairline)] pb-8">
        <div className="max-w-lg space-y-3">
          <FieldLabel htmlFor="country">{copy.countryLabel}</FieldLabel>

          <Controller
            name="country"
            control={control}
            render={({ field }) => (
              <>
                <input type="hidden" {...field} />
                <Select
                  value={field.value ?? ''}
                  onValueChange={(val) => field.onChange(val)}
                >
                  <SelectTrigger
                    id="country"
                    aria-required="true"
                    aria-invalid={Boolean(errors.country)}
                    className="w-full"
                  >
                    <SelectValue placeholder="Select jurisdiction" />
                  </SelectTrigger>
                  <SelectContent>
                    {JURISDICTION_OPTIONS.map((opt) => (
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

      {/* ── Attestation checkbox ─────────────────────────────────────────── */}
      <div className="border-b border-[var(--hairline)] py-8">
        <div className="max-w-lg space-y-3">
          <Controller
            name="is_business_entity"
            control={control}
            render={({ field }) => (
              <label
                htmlFor="is_business_entity"
                className="flex cursor-pointer items-start gap-3"
              >
                <input
                  id="is_business_entity"
                  type="checkbox"
                  aria-required="true"
                  aria-invalid={Boolean(errors.is_business_entity)}
                  checked={field.value === true}
                  onChange={(e) =>
                    field.onChange(e.target.checked ? true : undefined)
                  }
                  className="mt-0.5 h-4 w-4 shrink-0 cursor-pointer rounded-sm border border-[var(--hairline)] accent-[var(--moss-700)] focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] focus-visible:ring-offset-2"
                />
                <span
                  className="text-sm leading-relaxed text-[var(--ink-900)]"
                  style={{ fontFamily: "'Source Sans 3', sans-serif" }}
                >
                  {copy.checkboxLabel}
                </span>
              </label>
            )}
          />

          <FieldError message={errors.is_business_entity?.message} />
        </div>
      </div>

      {/* ── Submit ────────────────────────────────────────────────────────── */}
      <div className="pt-8">
        <button
          type="submit"
          disabled={
            !isBusinessEntityChecked ||
            isSubmitting ||
            signMutation.isPending
          }
          className="inline-flex h-11 items-center justify-center rounded-[6px] bg-[var(--ink-900)] px-6 text-sm font-medium text-[var(--background-elevated)] transition-colors hover:bg-[var(--ink-700)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50"
          style={{ fontFamily: "'Source Sans 3', sans-serif" }}
        >
          {signMutation.isPending ? 'Saving\u2026' : copy.submitCta}
        </button>
      </div>
    </form>
  )
}
