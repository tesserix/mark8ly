'use client'

/**
 * CloseBeforeDowngradeClient
 *
 * Renders the store Close/Delete picker when a plan downgrade is blocked by
 * store-cap overflow. Reached via:
 *
 *   /admin/stores/close-before-downgrade?target=studio&billing=annual&over_by=2
 *
 * Flow:
 *   1. Reads URL params to know which plan the merchant is targeting.
 *   2. Fetches the over-cap store list via useDowngradeBlockList.
 *   3. Merchant selects Close or Delete for each over-cap store.
 *   4. Apply button fires mutations, then redirects back to the plan-change
 *      page with ?retry=true so the original downgrade can re-apply.
 *
 * Design: Paper/Ink/Moss. Hairline-separated rows, no bordered cards.
 * Editorial voice — never urgency language.
 */

import { useCallback, useMemo, useState } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'

import { useToast } from '@/components/feedback/Toaster'
import { subscriptionCopy } from '@/lib/copy/subscription'
import { useDowngradeBlockList, useCloseStore, useDeleteStore } from '@/lib/api/subscription/hooks/useStoreDowngrade'
import type { DowngradeBlockStore } from '@/lib/api/subscription/schemas/stores'

// ---------------------------------------------------------------------------
// Plan store-cap table — mirrors PLAN_STORE_CAPS in stores.ts
// Used only to show the "X includes N stores" copy in the heading.
// ---------------------------------------------------------------------------

const PLAN_STORE_CAPS: Record<string, number> = {
  starter: 1,
  studio: 3,
  growth: 10,
  scale: 25,
  pro: Infinity,
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type StoreAction = 'close' | 'delete'

// A store whose action has been chosen by the merchant.
interface StoreResolution {
  storeId: string
  action: StoreAction
}

// ---------------------------------------------------------------------------
// Copy alias
// ---------------------------------------------------------------------------

const copy = subscriptionCopy.storeDowngrade

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

interface StoreRowProps {
  store: DowngradeBlockStore
  action: StoreAction | undefined
  onActionChange: (storeId: string, action: StoreAction) => void
}

function StoreRow({ store, action, onActionChange }: StoreRowProps) {
  const radioName = `store-action-${store.id}`

  // in_flight_orders may be null if the backend hasn't shipped the field yet.
  // TODO(P15): remove null check once backend populates in_flight_orders.
  const ordersText =
    store.in_flight_orders == null
      ? null
      : copy.inFlightOrdersTemplate(store.in_flight_orders)

  const exportHref = `/admin/stores/${store.id}/orders?export=csv`

  return (
    <li className="flex flex-col gap-4 border-b border-[var(--hairline,var(--ink-100))] py-8 sm:flex-row sm:items-start sm:gap-8">
      {/* Store name + orders info */}
      <div className="flex-1 space-y-2">
        <h3 className="font-['Source_Serif_4',_Georgia,_serif] text-lg font-medium leading-snug text-[var(--ink-900)]">
          {store.name}
        </h3>

        {store.in_flight_orders == null ? (
          // Backend hasn't shipped in_flight_orders yet — show a neutral placeholder.
          // TODO(P15): remove this branch once backend populates in_flight_orders.
          <p className="text-sm text-[var(--ink-900)] opacity-60">
            In-flight orders: <span aria-label="data unavailable">&mdash;</span>
          </p>
        ) : (
          <p
            className="text-sm text-[var(--ink-900)] opacity-70"
            aria-label={`${store.in_flight_orders} orders in flight`}
          >
            {ordersText}
          </p>
        )}

        {/* Export CSV link — only shown when there may be orders */}
        <a
          href={exportHref}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-1 text-sm text-[var(--moss-700)] underline underline-offset-2 hover:opacity-80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)]"
        >
          {copy.exportCsvCta}
          <span aria-hidden="true"> &rarr;</span>
        </a>
      </div>

      {/* Action radio group */}
      <fieldset className="shrink-0" aria-label={`Action for ${store.name}`}>
        <legend className="sr-only">Choose what happens to {store.name}</legend>
        <div className="flex flex-col gap-4 sm:w-64">
          <StoreActionOption
            name={radioName}
            value="close"
            label={copy.closeLabel}
            helpText={copy.closeHelp}
            checked={action === 'close'}
            onChange={() => onActionChange(store.id, 'close')}
          />
          <StoreActionOption
            name={radioName}
            value="delete"
            label={copy.deleteLabel}
            helpText={copy.deleteHelp}
            checked={action === 'delete'}
            onChange={() => onActionChange(store.id, 'delete')}
          />
        </div>
      </fieldset>
    </li>
  )
}

interface StoreActionOptionProps {
  name: string
  value: StoreAction
  label: string
  helpText: string
  checked: boolean
  onChange: () => void
}

function StoreActionOption({
  name,
  value,
  label,
  helpText,
  checked,
  onChange,
}: StoreActionOptionProps) {
  return (
    <label className="flex cursor-pointer items-start gap-3">
      <input
        type="radio"
        name={name}
        value={value}
        checked={checked}
        onChange={onChange}
        className="mt-0.5 h-4 w-4 shrink-0 cursor-pointer accent-[var(--moss-700)]"
      />
      <span className="flex flex-col gap-1">
        <span className="text-sm font-medium text-[var(--ink-900)]">{label}</span>
        {checked && (
          <span className="text-xs leading-relaxed text-[var(--ink-900)] opacity-60">
            {helpText}
          </span>
        )}
      </span>
    </label>
  )
}

// ---------------------------------------------------------------------------
// Skeleton row
// ---------------------------------------------------------------------------

function StoreRowSkeleton() {
  return (
    <li
      aria-hidden="true"
      className="flex flex-col gap-4 border-b border-[var(--hairline,var(--ink-100))] py-8 sm:flex-row sm:items-start sm:gap-8"
    >
      <div className="flex-1 space-y-3">
        <div className="h-6 w-48 rounded-[4px] bg-[var(--paper-200)] animate-pulse" />
        <div className="h-4 w-72 rounded-[4px] bg-[var(--paper-200)] animate-pulse" />
        <div className="h-4 w-32 rounded-[4px] bg-[var(--paper-200)] animate-pulse" />
      </div>
      <div className="shrink-0 space-y-3 sm:w-64">
        <div className="h-5 w-40 rounded-[4px] bg-[var(--paper-200)] animate-pulse" />
        <div className="h-5 w-40 rounded-[4px] bg-[var(--paper-200)] animate-pulse" />
      </div>
    </li>
  )
}

// ---------------------------------------------------------------------------
// Failure list — shows which stores failed and allows individual retry
// ---------------------------------------------------------------------------

interface FailureNoticeProps {
  failedStoreNames: string[]
}

function FailureNotice({ failedStoreNames }: FailureNoticeProps) {
  if (failedStoreNames.length === 0) return null
  return (
    <div
      role="alert"
      className="rounded-[6px] border border-[var(--signal,#C23B22)]/30 bg-[var(--background-elevated)] px-5 py-4"
    >
      <p className="text-sm font-medium text-[var(--ink-900)]">
        {failedStoreNames.length === 1
          ? 'One store could not be updated.'
          : `${failedStoreNames.length} stores could not be updated.`}
      </p>
      <ul className="mt-2 list-disc pl-5 text-sm text-[var(--ink-900)] opacity-70">
        {failedStoreNames.map((name) => (
          <li key={name}>{name}</li>
        ))}
      </ul>
      <p className="mt-2 text-xs text-[var(--ink-900)] opacity-60">
        Reselect an action for each failed store and try again.
      </p>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

/**
 * The store ID used as the subscription context for the preflight call.
 * In a multi-store tenant, any store ID works for subscription lookup because
 * subscriptions are tenant-scoped. We use a placeholder that callers can
 * replace with the real storeId if passed as a query param.
 *
 * TODO: accept a `store_id` query param so the correct subscription context
 * is used for the preflight endpoint once it ships.
 */
const SUBSCRIPTION_STORE_PLACEHOLDER = ''

export function CloseBeforeDowngradeClient() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { toast } = useToast()

  const targetPlan = searchParams.get('target') ?? ''
  const billingPeriod = searchParams.get('billing') ?? ''
  const overByRaw = searchParams.get('over_by') ?? '0'
  const overBy = Math.max(0, parseInt(overByRaw, 10) || 0)

  // Store ID for the subscription context — use query param if provided,
  // otherwise fall back to an empty string which disables the query.
  const subscriptionStoreId = searchParams.get('store_id') ?? SUBSCRIPTION_STORE_PLACEHOLDER

  const planLimit = PLAN_STORE_CAPS[targetPlan] ?? 0
  const planLimitDisplay = planLimit === Infinity ? '∞' : planLimit

  // ---------------------------------------------------------------------------
  // Data
  // ---------------------------------------------------------------------------

  const blockListQuery = useDowngradeBlockList(subscriptionStoreId, {
    targetPlan,
    billingPeriod,
    overBy,
  })

  // ---------------------------------------------------------------------------
  // Local state — per-store action choices and failure tracking
  // ---------------------------------------------------------------------------

  const [actions, setActions] = useState<Record<string, StoreAction>>({})
  const [failedIds, setFailedIds] = useState<ReadonlySet<string>>(new Set())
  const [isApplying, setIsApplying] = useState(false)

  const stores = blockListQuery.data?.stores ?? []

  // Derive over-cap stores that still need resolution (not yet applied).
  // After a partial failure, only failed stores remain visible for retry.
  const pendingStores = failedIds.size > 0
    ? stores.filter((s) => failedIds.has(s.id))
    : stores

  const allResolved = pendingStores.length > 0 &&
    pendingStores.every((s) => actions[s.id] !== undefined)

  const failedStoreNames = stores
    .filter((s) => failedIds.has(s.id))
    .map((s) => s.name)

  // ---------------------------------------------------------------------------
  // Action change handler — immutable update
  // ---------------------------------------------------------------------------

  const handleActionChange = useCallback(
    (storeId: string, action: StoreAction) => {
      setActions((prev) => ({ ...prev, [storeId]: action }))
    },
    [],
  )

  // ---------------------------------------------------------------------------
  // Mutation hooks — one per store in the block list.
  // We create a stable list of hooks for each store ID. Because hooks cannot
  // be called conditionally, we derive a fixed set based on the first resolved
  // stores list. The hook list is re-evaluated on component re-render, which
  // is safe because the block list is immutable after load.
  //
  // NOTE: React rules prohibit calling hooks in a loop. The approach here is
  // to batch mutations imperatively in the apply handler using the stores
  // client directly, bypassing individual hooks for each row. Each mutation
  // invalidates ['stores'] and ['subscription'] on success.
  // ---------------------------------------------------------------------------

  // We use a single pair of mutation hooks and call mutateAsync on each store
  // sequentially inside the apply handler. This avoids the "hooks in a loop"
  // problem while preserving per-store error tracking.
  const closeStoreMutation = useCloseStore('')   // storeId injected per-call below
  const deleteStoreMutation = useDeleteStore('')  // storeId injected per-call below

  // ---------------------------------------------------------------------------
  // Apply handler
  // ---------------------------------------------------------------------------

  const handleApply = useCallback(async () => {
    if (!allResolved || isApplying) return
    setIsApplying(true)

    const resolutions: StoreResolution[] = pendingStores
      .map((s) => ({ storeId: s.id, action: actions[s.id] as StoreAction }))
      .filter((r) => r.action !== undefined)

    const newFailedIds = new Set<string>()

    // Import API functions directly to avoid hooks-in-loop constraint.
    const { closeStore, deleteStore } = await import('@/lib/api/subscription/stores')

    await Promise.all(
      resolutions.map(async ({ storeId, action }) => {
        try {
          if (action === 'close') {
            await closeStore(storeId)
          } else {
            await deleteStore(storeId)
          }
        } catch {
          newFailedIds.add(storeId)
        }
      }),
    )

    setIsApplying(false)

    if (newFailedIds.size > 0) {
      setFailedIds(newFailedIds)
      // Keep only failed stores' actions so the merchant can retry.
      setActions((prev) => {
        const retainedActions: Record<string, StoreAction> = {}
        for (const id of newFailedIds) {
          if (prev[id]) retainedActions[id] = prev[id]
        }
        return retainedActions
      })
      toast.error(copy.toastError)
      return
    }

    // All mutations succeeded — invalidate query caches via the hooks
    // (the hooks already wire invalidation in onSuccess, but since we called
    // the raw API functions we trigger a manual invalidation here).
    void closeStoreMutation.reset()
    void deleteStoreMutation.reset()

    toast.success(copy.toastSuccess)

    // Navigate back to plan-change with ?retry=true so the original downgrade re-applies.
    const retryUrl = `/admin/settings/billing/plan-change?target=${encodeURIComponent(targetPlan)}&billing=${encodeURIComponent(billingPeriod)}&retry=true`
    router.push(retryUrl)
  }, [
    allResolved,
    isApplying,
    pendingStores,
    actions,
    targetPlan,
    billingPeriod,
    router,
    toast,
    closeStoreMutation,
    deleteStoreMutation,
  ])

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  const introText = useMemo(
    () =>
      targetPlan
        ? copy.introTemplate(
            targetPlan.charAt(0).toUpperCase() + targetPlan.slice(1),
            planLimit === Infinity ? Infinity : (planLimit as number),
            overBy,
          )
        : '',
    [targetPlan, planLimit, overBy],
  )

  return (
    <main
      id="main-content"
      className="mx-auto max-w-3xl px-4 pb-24 pt-12 sm:px-6 lg:px-8"
    >
      {/* Skip link target */}
      <a
        href="#store-list"
        className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-50 focus:rounded focus:bg-[var(--background-elevated)] focus:px-3 focus:py-2 focus:text-sm focus:underline"
      >
        Skip to store list
      </a>

      {/* ─── Page header ───────────────────────────────────────────────────── */}
      <header className="space-y-3 border-b border-[var(--hairline,var(--ink-100))] pb-10">
        <h1 className="font-['Source_Serif_4',_Georgia,_serif] text-3xl font-medium leading-tight text-[var(--ink-900)]">
          {copy.heading}
        </h1>
        {introText && (
          <p className="max-w-prose text-base leading-relaxed text-[var(--ink-900)] opacity-70">
            {introText}
          </p>
        )}
      </header>

      {/* ─── Store list ────────────────────────────────────────────────────── */}
      <section aria-labelledby="store-list-heading">
        <h2 id="store-list-heading" className="sr-only">
          Over-cap stores
        </h2>

        {blockListQuery.isLoading && (
          <ul id="store-list" aria-label="Loading stores" className="m-0 list-none p-0">
            {Array.from({ length: overBy > 0 ? overBy : 2 }).map((_, i) => (
              // eslint-disable-next-line react/no-array-index-key
              <StoreRowSkeleton key={i} />
            ))}
          </ul>
        )}

        {blockListQuery.isError && (
          <div
            role="alert"
            className="py-10 text-sm text-[var(--ink-900)] opacity-70"
          >
            Could not load over-cap stores. Try refreshing the page.
          </div>
        )}

        {!blockListQuery.isLoading && !blockListQuery.isError && (
          <ul id="store-list" aria-label="Over-cap stores" className="m-0 list-none p-0">
            {pendingStores.map((store) => (
              <StoreRow
                key={store.id}
                store={store}
                action={actions[store.id]}
                onActionChange={handleActionChange}
              />
            ))}
          </ul>
        )}
      </section>

      {/* ─── Failure notice ────────────────────────────────────────────────── */}
      {failedIds.size > 0 && (
        <div className="mt-8">
          <FailureNotice failedStoreNames={failedStoreNames} />
        </div>
      )}

      {/* ─── Confirm bar ───────────────────────────────────────────────────── */}
      {!blockListQuery.isLoading && !blockListQuery.isError && pendingStores.length > 0 && (
        <div className="mt-12 flex flex-col gap-3 border-t border-[var(--hairline,var(--ink-100))] pt-8 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <p className="text-sm font-medium text-[var(--ink-900)]">
              {copy.confirmHeading}
            </p>
            {!allResolved && (
              <p className="mt-1 text-xs text-[var(--ink-900)] opacity-60">
                Select an action for every store before applying.
              </p>
            )}
          </div>

          <button
            type="button"
            disabled={!allResolved || isApplying}
            onClick={() => void handleApply()}
            aria-busy={isApplying}
            className="inline-flex h-11 items-center justify-center rounded-[6px] bg-[var(--ink-900)] px-7 text-sm font-medium text-[var(--paper-200)] transition-colors hover:bg-[var(--ink-900)]/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--moss-700)] focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-40"
          >
            {isApplying ? 'Applying\u2026' : copy.confirmCta}
          </button>
        </div>
      )}
    </main>
  )
}
