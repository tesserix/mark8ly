# Rocket.Chat onboarding notifications

Status: accepted; production activation is controlled by the infrastructure repository.

## Decision

On successful initial tenant/store onboarding, enqueue
`rocketchat.onboarding_completed` in the same PostgreSQL transaction that creates
the tenant and store. Use the existing outbox drainer to deliver to the single
configured Rocket.Chat room. Ordinary logins, invited staff, additional stores,
unfinished sessions and failed transactions do not emit this event. Existing
analytics and onboarding-completed events remain unchanged.

The event is an explicit allowlist: owner name/email, business name, tenant/store
IDs, store slug, completion timestamp, country/currency/timezone, classification
and generated admin/storefront links. Never serialize the request or draft;
passwords, tax identifiers, cookies, tokens and drafts are excluded. User text is
bounded and escaped; standalone mentions are neutralized. URLs are restricted to
HTTPS mark8ly.com hosts. These events contain customer contact information and
must follow the existing database/backup retention and access policies.

## Delivery and failure behavior

Use HTTPS REST `chat.sendMessage` with ID `mark8ly-onboarding-<tenant UUID>`.
Before sending, `chat.getMessage` checks whether that ID already exists. Only a
matching room, bot author and message counts as delivered. This handles replay
and a lost HTTP response after the server accepted a message without duplicating
it. A retry may perform another send attempt, but the unique message ID prevents
another stored message. Conflicting IDs fail visibly rather than being accepted.

Requests have a five-second timeout and a ten-second per-event deadline, obey
cancellation, reject redirects and cap response decoding. Errors contain neither
credentials, response bodies nor customer details. Transient failures return to
the existing exponential-backoff queue; terminal failures remain dead-letter
events for operator inspection and replay. The existing retry transition needed
`status=pending`; otherwise failed jobs stayed in flight until crash recovery.

There is no synchronous chat call in signup, new service, schema migration,
queue, disk or inbound endpoint. Disabled replicas still register a retryable
handler so a rolling enablement does not dead-letter newly queued events as an
unknown kind. Deploy this compatible binary everywhere before enabling signup
notifications. Disabled by default in application code. When
enabled, incomplete bot configuration fails startup rather than silently losing
notifications. Production Helm enables it only as part of the approved rollout.

## Security and operations

Use a dedicated non-admin service identity subscribed only to
`onboarding-mark8ly`, using the built-in bot role restricted to joined-room visibility. Community
edition does not support paid custom roles. Remove the stock bot role's room creation, user administration,
impersonation or access to unjoined rooms. Verify these restrictions against the
live API before enabling production. Human access remains the existing Zitadel
allowlist. Secrets are delivered from OpenBao through ESO, never browser code.

Production rollout order, monitoring and rollback are documented in the
infrastructure repository's `docs/mark8ly-rocketchat-onboarding.md`. This change
does not backfill historical signups or enable support-channel integrations.

## Verification

Unit tests cover formatted details, escaping, replay deduplication, mismatched
author rejection, redacted failures, invalid configuration, cancellation and
redirect denial. Database tests cover atomic completion, repeated completion,
secret exclusion and rollback. The existing onboarding/outbox integration suites
run serially against a disposable PostgreSQL database because both truncate the
same tables. Older onboarding test fixtures now provide their required store
repository. Race tests, build, vet and lint of changed code pass.
