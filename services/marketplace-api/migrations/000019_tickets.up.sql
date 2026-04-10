CREATE TABLE tickets (
    id                UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         UUID          NOT NULL,
    store_id          UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    ticket_number     VARCHAR(20)   NOT NULL,
    subject           VARCHAR(300)  NOT NULL,
    description       TEXT          NOT NULL,
    status            VARCHAR(20)   NOT NULL DEFAULT 'open',
    priority          VARCHAR(10)   NOT NULL DEFAULT 'medium',
    submitted_by_name VARCHAR(200)  NOT NULL,
    submitted_by_email VARCHAR(300) NOT NULL,
    resolved_at       TIMESTAMPTZ,
    created_at        TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id, ticket_number)
);
CREATE INDEX tickets_store_status_idx ON tickets (store_id, status);

CREATE TABLE ticket_replies (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id       UUID          NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    author_type     VARCHAR(20)   NOT NULL,
    author_name     VARCHAR(200)  NOT NULL,
    author_email    VARCHAR(300),
    content         TEXT          NOT NULL,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX tr_ticket_idx ON ticket_replies (ticket_id);
