# Merchant testimonials

Store testimonials belong to platform-api, alongside the canonical stores and
tenants. One row per store makes PUT retries idempotent. Planning assumptions:
20 peak public reads/s, under one write/s, 100:1 reads to writes, at most 10,000
rows in year one and 100,000 in year three, each below 5 KB. Target 99.9%
availability and API p99 below 300 ms; each dependency has a three-second bound.
No additional service or cache is needed for this volume.

The admin BFF derives the store and user from its authenticated session. Internal
GET/PUT routes require the strict shared-secret guard and independently check
OpenFGA's store settings permission. Missing authorization fails closed. The
assets are merchant identity and consent; another tenant or an unauthenticated
caller must never edit a review or publish on someone else's behalf.

PUT validates name (1–100 characters), quote (1–1000), and rating (1–5). Publishing
requires explicit merchant consent for this version of the text. Saving as a
draft withdraws publication. The merchant's explicit submission is approval;
there is no platform moderation queue. Draft seed text is not customer feedback
and is never published automatically. Seeds only match a unique existing store
name, never guessed identifiers, and never overwrite a submitted review.

The public endpoint returns up to twelve published, consented reviews from active
stores and tenants, without internal identity or consent metadata. All star
ratings qualify equally. The static homepage loads them through a same-origin
uncached route after mount, preserving its current edge caching. Read errors or
an empty list omit the section; admin errors preserve the form and show a retry
message. Publication/withdrawal appears on the next public fetch or page load.

Migration 22 adds a table and partial index, without changing existing rows or
tables. A disposable database verifies up/down/up and the public/private flow.
Deploy the migration before the new API (the existing migrate init container does
this). The previous API image asserts schema 21 and cannot restart after schema
22 is applied. Recovery requires a forward fix or a compatibility build that
retains schema 22; an image-only rollback is insufficient. The down migration
drops testimonial data and is only for disposable test databases. No production
rollback or destructive down is authorized by this design. Cost is small
Postgres storage and one bounded read per homepage visit; no new paid services.
