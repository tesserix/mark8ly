-- 000010_gift_cards.up.sql
-- Marketing M2: Gift cards with transaction ledger.

CREATE TABLE gift_cards (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    store_id        UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    code            VARCHAR(50)   NOT NULL,
    initial_balance NUMERIC(12,2) NOT NULL,
    current_balance NUMERIC(12,2) NOT NULL,
    currency_code   CHAR(3)       NOT NULL,
    status          VARCHAR(20)   NOT NULL DEFAULT 'active',
    sender_name     VARCHAR(200),
    sender_email    VARCHAR(300),
    recipient_name  VARCHAR(200),
    recipient_email VARCHAR(300),
    message         TEXT,
    purchased_at    TIMESTAMPTZ,
    expires_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id, code),
    CHECK (current_balance >= 0),
    CHECK (initial_balance > 0)
);
CREATE INDEX gift_cards_store_status_idx ON gift_cards (store_id, status);
CREATE INDEX gift_cards_tenant_idx ON gift_cards (tenant_id);

CREATE TABLE gift_card_transactions (
    id              UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID          NOT NULL,
    gift_card_id    UUID          NOT NULL REFERENCES gift_cards(id) ON DELETE CASCADE,
    order_id        UUID,
    type            VARCHAR(20)   NOT NULL,
    amount          NUMERIC(12,2) NOT NULL,
    balance_after   NUMERIC(12,2) NOT NULL,
    note            VARCHAR(200),
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT now(),
    CHECK (balance_after >= 0)
);
CREATE INDEX gc_txn_card_idx ON gift_card_transactions (gift_card_id);
CREATE INDEX gc_txn_tenant_idx ON gift_card_transactions (tenant_id);
