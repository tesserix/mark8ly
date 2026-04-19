'use client'

/**
 * SurveyStep — exit survey (step 2 of the cancellation wizard).
 *
 * Renders a radiogroup with 6 reason options and an optional feedback
 * textarea. Validates with React Hook Form + Zod. Passes {reason, feedback}
 * to onContinue on submit.
 */
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useRouter } from 'next/navigation'
import { cancellationCopy } from '@/lib/copy/cancellation'

const copy = cancellationCopy.surveyStep

const reasonCodes = copy.reasons.map((r) => r.code) as [string, ...string[]]

const surveySchema = z.object({
  reason: z.enum(reasonCodes, { error: 'Please select a reason.' }),
  feedback: z.string().max(500).optional(),
})

type SurveyFields = z.infer<typeof surveySchema>

interface SurveyStepProps {
  onContinue: (reason: string, feedback: string) => void
}

export function SurveyStep({ onContinue }: SurveyStepProps) {
  const router = useRouter()

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<SurveyFields>({
    resolver: zodResolver(surveySchema),
    defaultValues: { reason: undefined, feedback: '' },
  })

  function onSubmit(values: SurveyFields) {
    onContinue(values.reason, values.feedback ?? '')
  }

  return (
    <section aria-labelledby="survey-heading">
      <h1
        id="survey-heading"
        className="mb-2 font-serif text-2xl font-normal text-[var(--ink-900)]"
      >
        {copy.title}
      </h1>

      <p className="mb-6 text-sm leading-relaxed text-[var(--ink-700)]">
        {copy.body}
      </p>

      <form onSubmit={handleSubmit(onSubmit)} noValidate>
        <fieldset>
          <legend className="sr-only">Reason for cancelling</legend>

          <div
            role="radiogroup"
            aria-required="true"
            aria-label="Reason for cancelling"
            className="mb-6 flex flex-col gap-3"
          >
            {copy.reasons.map(({ code, label }) => (
              <label
                key={code}
                className="flex cursor-pointer items-center gap-3 text-sm text-[var(--ink-900)]"
              >
                <input
                  type="radio"
                  value={code}
                  {...register('reason')}
                  className="h-4 w-4 cursor-pointer accent-[var(--moss-700)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]"
                />
                {label}
              </label>
            ))}
          </div>

          {errors.reason && (
            <p role="alert" className="mb-4 text-xs text-[var(--danger)]">
              {errors.reason.message}
            </p>
          )}
        </fieldset>

        <div className="mb-8">
          <label
            htmlFor="survey-feedback"
            className="mb-2 block text-sm text-[var(--ink-700)]"
          >
            {copy.feedbackLabel}
          </label>
          <textarea
            id="survey-feedback"
            rows={3}
            placeholder={copy.feedbackPlaceholder}
            {...register('feedback')}
            className="w-full resize-none rounded-[6px] border border-[var(--hairline,var(--ink-100))] bg-[var(--background-elevated)] px-3 py-2 text-sm text-[var(--ink-900)] placeholder:text-[var(--ink-400)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]"
          />
          {errors.feedback && (
            <p role="alert" className="mt-1 text-xs text-[var(--danger)]">
              {errors.feedback.message}
            </p>
          )}
        </div>

        <div className="flex flex-col gap-3 sm:flex-row sm:gap-4">
          <button
            type="submit"
            className="inline-flex h-10 items-center rounded-[6px] bg-[var(--ink-900)] px-6 text-sm font-medium text-[var(--paper-200)] transition-colors hover:bg-[var(--ink-700)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]"
          >
            {copy.submitCta}
          </button>

          <button
            type="button"
            onClick={() => router.push('/settings/billing')}
            className="inline-flex h-10 items-center rounded-[6px] border border-[var(--hairline,var(--ink-100))] bg-[var(--background-elevated)] px-6 text-sm font-medium text-[var(--ink-900)] transition-colors hover:bg-[var(--paper-200)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]"
          >
            Keep my subscription
          </button>
        </div>
      </form>
    </section>
  )
}
