'use client'

/**
 * CancelWizard — 4-step cancellation state machine.
 *
 * Steps: confirm → survey → save_offer (conditional) → final
 *
 * Eligibility for save_offer step:
 *   Show for reasons in SAVE_OFFER_ELIGIBLE_REASONS (§15.1).
 *   Skip for 'going_out_of_business' and 'switching_to'.
 */
import { useReducer } from 'react'
import { SAVE_OFFER_ELIGIBLE_REASONS } from '@/lib/copy/cancellation'
import { ConfirmStep } from './ConfirmStep'
import { SurveyStep } from './SurveyStep'
import { SaveOfferStep } from './SaveOfferStep'
import { FinalConfirmStep } from './FinalConfirmStep'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type WizardStep = 'confirm' | 'survey' | 'save_offer' | 'final'

interface WizardState {
  step: WizardStep
  reason: string
  feedback: string
}

type WizardAction =
  | { type: 'ADVANCE_TO_SURVEY' }
  | { type: 'ADVANCE_FROM_SURVEY'; reason: string; feedback: string }
  | { type: 'ADVANCE_TO_FINAL' }

function wizardReducer(state: WizardState, action: WizardAction): WizardState {
  switch (action.type) {
    case 'ADVANCE_TO_SURVEY':
      return { ...state, step: 'survey' }

    case 'ADVANCE_FROM_SURVEY': {
      const nextStep = SAVE_OFFER_ELIGIBLE_REASONS.has(action.reason)
        ? 'save_offer'
        : 'final'
      return { ...state, step: nextStep, reason: action.reason, feedback: action.feedback }
    }

    case 'ADVANCE_TO_FINAL':
      return { ...state, step: 'final' }

    default:
      return state
  }
}

const initialState: WizardState = {
  step: 'confirm',
  reason: '',
  feedback: '',
}

// ---------------------------------------------------------------------------
// Progress dots
// ---------------------------------------------------------------------------

const STEPS: WizardStep[] = ['confirm', 'survey', 'save_offer', 'final']

interface ProgressDotsProps {
  current: WizardStep
}

function ProgressDots({ current }: ProgressDotsProps) {
  const currentIndex = STEPS.indexOf(current)

  return (
    <div
      role="progressbar"
      aria-valuenow={currentIndex + 1}
      aria-valuemin={1}
      aria-valuemax={STEPS.length}
      aria-label="Cancellation wizard progress"
      className="mb-8 flex items-center gap-2"
    >
      {STEPS.map((step, i) => {
        const isCompleted = i < currentIndex
        const isCurrent = i === currentIndex

        return (
          <span
            key={step}
            aria-hidden="true"
            className={[
              'h-2 w-2 rounded-full transition-colors',
              isCompleted
                ? 'bg-[var(--moss-700)]'
                : isCurrent
                  ? 'border border-[var(--ink-900)] bg-transparent'
                  : 'border border-[var(--ink-400,var(--ink-300))] bg-transparent',
            ].join(' ')}
          />
        )
      })}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Wizard
// ---------------------------------------------------------------------------

interface CancelWizardProps {
  storeId: string
}

export function CancelWizard({ storeId }: CancelWizardProps) {
  const [state, dispatch] = useReducer(wizardReducer, initialState)

  return (
    <div className="mx-auto max-w-lg py-10">
      <ProgressDots current={state.step} />

      {state.step === 'confirm' && (
        <ConfirmStep
          storeId={storeId}
          onContinue={() => dispatch({ type: 'ADVANCE_TO_SURVEY' })}
        />
      )}

      {state.step === 'survey' && (
        <SurveyStep
          onContinue={(reason, feedback) =>
            dispatch({ type: 'ADVANCE_FROM_SURVEY', reason, feedback })
          }
        />
      )}

      {state.step === 'save_offer' && (
        <SaveOfferStep
          storeId={storeId}
          onDecline={() => dispatch({ type: 'ADVANCE_TO_FINAL' })}
        />
      )}

      {state.step === 'final' && (
        <FinalConfirmStep
          storeId={storeId}
          reason={state.reason}
          feedback={state.feedback}
        />
      )}
    </div>
  )
}
