# Merchant testimonials

Store testimonials belong to platform-api, alongside the canonical stores and
tenants. A unique store key makes PUT retries idempotent. Planning assumptions:
20 peak public reads/s, under one write/s, 100:1 reads to writes, at most 10,000
rows in year one and 100,000 in year three, each below 5 KB. Target 99.9%
availability and API p99 below 300 ms; each dependency has a three-second bound.
No additional service or cache is needed for this volume.

The admin BFF derives the store and user from its authenticated session. Internal
GET/PUT routes require the strict shared-secret guard and independently check
OpenFGA's store settings permission. Missing authorization fails closed. The
assets are merchant identity and consent; another tenant or an unauthenticated
caller must never edit a review or publish on someone else's behalf.

PUT validates quote (1–1000 characters) and rating (1–5). Publishing
requires explicit merchant consent for this version of the text. Saving as a
draft withdraws publication. The merchant's explicit submission is approval;
there is no platform moderation queue. Draft seed text is not customer feedback
and is never published automatically. Seeds only match a unique existing store
name, never guessed identifiers, and never overwrite a submitted review.

The public endpoint returns up to twelve published, consented reviews from active
stores and tenants. It includes only store name and store role for attribution,
never personal names or internal identity/consent metadata. The role is derived
from the canonical tenant owner and authorized submitter, never form input. All star
ratings qualify equally. The static homepage loads them through a same-origin
uncached route after mount, preserving its current edge caching. Read errors or
an empty list omit the section; admin errors preserve the form and show a retry
message. Publication/withdrawal appears on the next public fetch or page load.

Migration 23 allows internal proposed copy before a store has registered. Each
row has its own UUID, an optional store FK, and an internal store-name label.
The store key remains unique, preserving existing merchant upserts. Unlinked
rows are constrained to stay unpublished and the public feed's store join
excludes them. The four requested store drafts are seeded without personal
names, with an existing unique store linked where available; existing reviews
are never overwritten. The Bondi Store's merchant can review its private draft
in admin. The other three stay internal until their stores exist and are linked.
The down migration removes unlinked drafts and preserves linked merchant rows;
it is destructive and is tested only in disposable databases.

Migration 24 lets an unlinked row be published for stores onboarded offline
before registration. The store's manager supplies consent on the owner's behalf,
and the row still needs a store name. The public feed shows linked rows only for
active stores and tenants, and shows unlinked rows by store name. Its down
migration unpublishes every unlinked row before restoring the old constraint.

Migration 22 adds a table and partial index, without changing existing rows or
tables. A disposable database verifies up/down/up and the public/private flow.
Deploy the migration before the new API (the existing migrate init container does
this). The preceding API image asserts its exact schema version and cannot
restart after the next migration is applied. Recovery requires a forward fix
or a compatibility build retaining the new schema version; an image-only
rollback is insufficient. The down migration
drops testimonial data and is only for disposable test databases. No production
rollback or destructive down is authorized by this design. Cost is small
Postgres storage and one bounded read per homepage visit; no new paid services.
