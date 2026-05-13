-- Customer-support tickets for mark8ly storefronts. Distinct from the
-- platform-engineering `tickets` table; this is the one the
-- marketplace-api ticket package backs (TableName() returns
-- support_tickets). The schema mirrors the GORM model in
-- services/marketplace-api/internal/ticket/models.go one-for-one.
--
-- The conversation_id column was added at creation time (instead of as
-- a separate migration) because the AI escalation flow that spawned
-- this table is the same delivery as the table itself; there's no
-- prior production data to back-fill.

CREATE TABLE IF NOT EXISTS public.support_tickets (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL,
    store_id            uuid NOT NULL,
    ticket_number       varchar(20) NOT NULL,
    subject             varchar(300) NOT NULL,
    description         text NOT NULL,
    status              varchar(20) NOT NULL DEFAULT 'open',
    priority            varchar(10) NOT NULL DEFAULT 'medium',
    submitted_by_name   varchar(200) NOT NULL,
    submitted_by_email  varchar(300) NOT NULL,
    -- conversation_id links the ticket back to the Otto chat that
    -- spawned it (NULL for tickets opened manually by a merchant).
    conversation_id     varchar(64),
    resolved_at         timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT support_tickets_status_chk
        CHECK (status IN ('open','in_progress','resolved','closed')),
    CONSTRAINT support_tickets_priority_chk
        CHECK (priority IN ('low','medium','high'))
);

-- Per-store ticket-number uniqueness so the visible ID a customer
-- quotes ("Case #ST-042") is unambiguous within the store.
CREATE UNIQUE INDEX IF NOT EXISTS idx_support_tickets_store_number
    ON public.support_tickets (store_id, ticket_number);

-- Admin inbox list query: latest first, scoped to (tenant, store, status).
CREATE INDEX IF NOT EXISTS idx_support_tickets_store_status_created
    ON public.support_tickets (store_id, tenant_id, status, created_at DESC);

-- Idempotency lookup for the slm-router escalation hook ("did this
-- conversation already mint a ticket?"). Partial index — most rows
-- are NULL.
CREATE INDEX IF NOT EXISTS idx_support_tickets_conversation
    ON public.support_tickets (store_id, conversation_id)
    WHERE conversation_id IS NOT NULL;

-- Customer "list my tickets by email" path (storefront My Cases page).
CREATE INDEX IF NOT EXISTS idx_support_tickets_store_email_created
    ON public.support_tickets (store_id, submitted_by_email, created_at DESC);

CREATE TABLE IF NOT EXISTS public.support_ticket_replies (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id     uuid NOT NULL REFERENCES public.support_tickets(id)
                       ON DELETE CASCADE,
    author_type   varchar(20) NOT NULL,
    author_name   varchar(200) NOT NULL,
    author_email  varchar(300),
    content       text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT support_ticket_replies_author_chk
        CHECK (author_type IN ('merchant','platform','customer'))
);

CREATE INDEX IF NOT EXISTS idx_support_ticket_replies_ticket_created
    ON public.support_ticket_replies (ticket_id, created_at ASC);
