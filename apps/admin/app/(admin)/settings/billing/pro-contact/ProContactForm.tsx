'use client'

/**
 * ProContactForm — Pro contact-sales enquiry form.
 *
 * Layout (editorial, left-aligned):
 *   - Intro paragraph (Source Sans 3)
 *   - 2-column grid at md (company name / contact name side-by-side)
 *   - Single-column on mobile
 *   - Hairline between field groups
 *   - Submit CTA row
 *
 * Behaviour:
 *   - RHF + zodResolver validates the form client-side.
 *   - On submit, delegates to useSubmitProContact.
 *   - On success (backend live):     toast success + redirect to /settings/billing.
 *   - On success (fallback='mailto'): build mailto href, open via window.location,
 *                                     toast mailtoFallback, stay on page.
 *   - On error:                       toast error with sales email, stay on page.
 *
 * Accessibility:
 *   - Every field has a visible <label> associated via htmlFor/id.
 *   - aria-invalid on fields with validation errors.
 *   - aria-describedby pointing to the error message span.
 *   - Keyboard navigation correct (no div traps).
 */

import { useId } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useRouter } from 'next/navigation'
import { z } from 'zod'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@tesserix/web'

const SELECT_UNSET = '__unset__'
import {
  proContactRequestSchema,
  type ProContactRequest,
} from '@/lib/api/subscription/schemas/proContact'
import { useSubmitProContact } from '@/lib/api/subscription/hooks/useProContact'
import { useToast } from '@/components/feedback/Toaster'
import { subscriptionCopy } from '@/lib/copy/subscription'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface ProContactFormProps {
  storeId: string
}

/**
 * FormValues = the raw input shape RHF manages (before Zod transforms).
 * migration_timeline includes "" because the HTML select emits "" when
 * no option is selected. The schema's .transform() coerces "" → undefined
 * before the validated value is sent to the API.
 */
type FormValues = z.input<typeof proContactRequestSchema>

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const copy = subscriptionCopy.proContact

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function buildMailtoHref(values: ProContactRequest): string {
  const subject = encodeURIComponent('Pro inquiry')
  const bodyLines = [
    `Company: ${values.company_name}`,
    `Name: ${values.contact_name}`,
    `Email: ${values.contact_email}`,
    values.phone ? `Phone: ${values.phone}` : null,
    `Monthly orders: ${values.monthly_orders}`,
    values.current_platform
      ? `Current platform: ${values.current_platform}`
      : null,
    values.migration_timeline
      ? `Timeline: ${values.migration_timeline}`
      : null,
    values.notes ? `Notes: ${values.notes}` : null,
  ]
  const body = encodeURIComponent(
    bodyLines.filter(Boolean).join('\n'),
  )
  return `mailto:${copy.salesEmail}?subject=${subject}&body=${body}`
}

// ---------------------------------------------------------------------------
// Field components — thin wrappers that wire label + aria attributes.
// These are colocated here because they are page-specific (not shared).
// ---------------------------------------------------------------------------

interface FieldProps {
  id: string
  label: string
  error?: string
  children: React.ReactNode
}

function Field({ id, label, error, children }: FieldProps) {
  const errorId = `${id}-error`
  return (
    <div className="flex flex-col gap-1.5">
      <label
        htmlFor={id}
        className="text-sm font-medium text-[var(--ink-900)]"
      >
        {label}
      </label>
      {children}
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
// ProContactForm
// ---------------------------------------------------------------------------

export function ProContactForm({ storeId }: ProContactFormProps) {
  const router = useRouter()
  const { toast } = useToast()
  const mutation = useSubmitProContact(storeId)

  const formId = useId()

  const {
    register,
    control,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({
    resolver: zodResolver(proContactRequestSchema),
    defaultValues: {
      company_name: '',
      contact_name: '',
      contact_email: '',
      phone: '',
      monthly_orders: 0,
      current_platform: '',
      migration_timeline: undefined,
      notes: '',
    },
  })

  const isPending = isSubmitting || mutation.isPending

  const onSubmit = (values: FormValues) => {
    // Cast to ProContactRequest — the zodResolver has already run the schema
    // transforms (empty string → undefined) before this handler is called.
    const validated = values as ProContactRequest
    mutation.mutate(validated, {
      onSuccess: (result) => {
        if (result.fallback === 'mailto') {
          const href = buildMailtoHref(validated)
          window.location.href = href
          toast.info(copy.toastMailtoFallback)
          return
        }
        toast.success(copy.toastSuccess)
        router.push('/settings/billing')
      },
      onError: () => {
        toast.error(copy.toastError)
      },
    })
  }

  // -------------------------------------------------------------------------
  // Field IDs (stable across renders via useId)
  // -------------------------------------------------------------------------
  const ids = {
    companyName: `${formId}-company-name`,
    contactName: `${formId}-contact-name`,
    contactEmail: `${formId}-contact-email`,
    phone: `${formId}-phone`,
    monthlyOrders: `${formId}-monthly-orders`,
    currentPlatform: `${formId}-current-platform`,
    migrationTimeline: `${formId}-migration-timeline`,
    notes: `${formId}-notes`,
  }

  return (
    <form
      onSubmit={handleSubmit(onSubmit)}
      noValidate
      aria-label={copy.heading}
      className="max-w-2xl space-y-0"
    >
      {/* ── Identity fields: 2-col at md ─────────────────────────────── */}
      <fieldset className="border-0 p-0 m-0">
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <Field
            id={ids.companyName}
            label={copy.labels.companyName}
            error={errors.company_name?.message}
          >
            <input
              id={ids.companyName}
              type="text"
              autoComplete="organization"
              aria-invalid={errors.company_name ? true : undefined}
              aria-describedby={
                errors.company_name ? `${ids.companyName}-error` : undefined
              }
              className="h-9 w-full rounded-md border border-[var(--hairline)] bg-[var(--background-elevated)] px-3 text-sm text-[var(--ink-900)] placeholder:text-[var(--ink-400)] focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)] focus:ring-offset-1 disabled:opacity-50"
              {...register('company_name')}
            />
          </Field>

          <Field
            id={ids.contactName}
            label={copy.labels.contactName}
            error={errors.contact_name?.message}
          >
            <input
              id={ids.contactName}
              type="text"
              autoComplete="name"
              aria-invalid={errors.contact_name ? true : undefined}
              aria-describedby={
                errors.contact_name ? `${ids.contactName}-error` : undefined
              }
              className="h-9 w-full rounded-md border border-[var(--hairline)] bg-[var(--background-elevated)] px-3 text-sm text-[var(--ink-900)] placeholder:text-[var(--ink-400)] focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)] focus:ring-offset-1 disabled:opacity-50"
              {...register('contact_name')}
            />
          </Field>
        </div>

        {/* ── Contact details: 2-col at md ──────────────────────────── */}
        <div className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
          <Field
            id={ids.contactEmail}
            label={copy.labels.contactEmail}
            error={errors.contact_email?.message}
          >
            <input
              id={ids.contactEmail}
              type="email"
              autoComplete="email"
              aria-invalid={errors.contact_email ? true : undefined}
              aria-describedby={
                errors.contact_email
                  ? `${ids.contactEmail}-error`
                  : undefined
              }
              className="h-9 w-full rounded-md border border-[var(--hairline)] bg-[var(--background-elevated)] px-3 text-sm text-[var(--ink-900)] placeholder:text-[var(--ink-400)] focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)] focus:ring-offset-1 disabled:opacity-50"
              {...register('contact_email')}
            />
          </Field>

          <Field
            id={ids.phone}
            label={copy.labels.phone}
            error={errors.phone?.message}
          >
            <input
              id={ids.phone}
              type="tel"
              autoComplete="tel"
              aria-invalid={errors.phone ? true : undefined}
              aria-describedby={
                errors.phone ? `${ids.phone}-error` : undefined
              }
              className="h-9 w-full rounded-md border border-[var(--hairline)] bg-[var(--background-elevated)] px-3 text-sm text-[var(--ink-900)] placeholder:text-[var(--ink-400)] focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)] focus:ring-offset-1 disabled:opacity-50"
              {...register('phone')}
            />
          </Field>
        </div>
      </fieldset>

      {/* ── Hairline ─────────────────────────────────────────────────── */}
      <hr className="my-6 border-t border-[var(--hairline)]" />

      {/* ── Store details ────────────────────────────────────────────── */}
      <fieldset className="border-0 p-0 m-0 space-y-4">
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <Field
            id={ids.monthlyOrders}
            label={copy.labels.monthlyOrders}
            error={errors.monthly_orders?.message}
          >
            <input
              id={ids.monthlyOrders}
              type="number"
              inputMode="numeric"
              min={0}
              aria-invalid={errors.monthly_orders ? true : undefined}
              aria-describedby={
                errors.monthly_orders
                  ? `${ids.monthlyOrders}-error`
                  : `${ids.monthlyOrders}-help`
              }
              className="h-9 w-full rounded-md border border-[var(--hairline)] bg-[var(--background-elevated)] px-3 text-sm text-[var(--ink-900)] placeholder:text-[var(--ink-400)] focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)] focus:ring-offset-1 disabled:opacity-50"
              {...register('monthly_orders', { valueAsNumber: true })}
            />
            {!errors.monthly_orders && (
              <span
                id={`${ids.monthlyOrders}-help`}
                className="text-xs text-[var(--ink-500)]"
              >
                {copy.monthlyOrdersHelp}
              </span>
            )}
          </Field>

          <Field
            id={ids.currentPlatform}
            label={copy.labels.currentPlatform}
            error={errors.current_platform?.message}
          >
            <input
              id={ids.currentPlatform}
              type="text"
              placeholder="Shopify, WooCommerce, etc."
              aria-invalid={errors.current_platform ? true : undefined}
              aria-describedby={
                errors.current_platform
                  ? `${ids.currentPlatform}-error`
                  : undefined
              }
              className="h-9 w-full rounded-md border border-[var(--hairline)] bg-[var(--background-elevated)] px-3 text-sm text-[var(--ink-900)] placeholder:text-[var(--ink-400)] focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)] focus:ring-offset-1 disabled:opacity-50"
              {...register('current_platform')}
            />
          </Field>
        </div>

        <Field
          id={ids.migrationTimeline}
          label={copy.labels.migrationTimeline}
          error={errors.migration_timeline?.message}
        >
          <Controller
            control={control}
            name="migration_timeline"
            render={({ field }) => (
              <Select
                value={field.value && field.value.length > 0 ? field.value : SELECT_UNSET}
                onValueChange={(next) =>
                  field.onChange(next === SELECT_UNSET ? '' : next)
                }
              >
                <SelectTrigger
                  id={ids.migrationTimeline}
                  aria-invalid={errors.migration_timeline ? true : undefined}
                  aria-describedby={
                    errors.migration_timeline
                      ? `${ids.migrationTimeline}-error`
                      : undefined
                  }
                  className="w-full"
                >
                  <SelectValue placeholder="Select an option" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={SELECT_UNSET}>Select an option</SelectItem>
                  {copy.timelineOptions.map((opt) => (
                    <SelectItem key={opt.value} value={opt.value}>
                      {opt.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          />
        </Field>
      </fieldset>

      {/* ── Hairline ─────────────────────────────────────────────────── */}
      <hr className="my-6 border-t border-[var(--hairline)]" />

      {/* ── Notes ────────────────────────────────────────────────────── */}
      <Field
        id={ids.notes}
        label={copy.labels.notes}
        error={errors.notes?.message}
      >
        <textarea
          id={ids.notes}
          rows={4}
          aria-invalid={errors.notes ? true : undefined}
          aria-describedby={
            errors.notes ? `${ids.notes}-error` : undefined
          }
          className="w-full resize-y rounded-md border border-[var(--hairline)] bg-[var(--background-elevated)] px-3 py-2 text-sm text-[var(--ink-900)] placeholder:text-[var(--ink-400)] focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)] focus:ring-offset-1 disabled:opacity-50"
          {...register('notes')}
        />
      </Field>

      {/* ── Submit ───────────────────────────────────────────────────── */}
      <div className="mt-8 flex items-center gap-4">
        <button
          type="submit"
          disabled={isPending}
          className="inline-flex h-9 items-center justify-center rounded-md bg-[var(--ink-900)] px-5 text-sm font-medium text-[var(--paper-200)] transition-colors hover:bg-[var(--moss-700)] focus:outline-none focus:ring-2 focus:ring-[var(--moss-700)] focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {isPending ? copy.submittingCta : copy.submitCta}
        </button>
      </div>
    </form>
  )
}
