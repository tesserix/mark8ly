-- Reference data for the location domain.
-- Read-only at runtime; populated by cmd/seed from seed/locations.json.

CREATE TABLE countries (
    code          CHAR(2)       PRIMARY KEY,
    name          VARCHAR(100)  NOT NULL,
    native_name   VARCHAR(100)  NOT NULL DEFAULT '',
    calling_code  VARCHAR(10)   NOT NULL DEFAULT '',
    currency_code CHAR(3)       NOT NULL DEFAULT '',
    flag_emoji    VARCHAR(10)   NOT NULL DEFAULT '',
    region        VARCHAR(50)   NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE TABLE states (
    id           VARCHAR(10)  PRIMARY KEY,           -- "US-CA", "IN-MH"
    country_code CHAR(2)      NOT NULL,
    code         VARCHAR(10)  NOT NULL,              -- "CA", "MH"
    name         VARCHAR(100) NOT NULL,
    type         VARCHAR(20)  NOT NULL DEFAULT 'state',
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    CONSTRAINT states_country_fk
        FOREIGN KEY (country_code) REFERENCES countries(code)
        ON DELETE RESTRICT
);

CREATE INDEX idx_states_country_code ON states (country_code);

CREATE TABLE currencies (
    code           CHAR(3)      PRIMARY KEY,         -- "USD", "INR"
    name           VARCHAR(100) NOT NULL,
    symbol         VARCHAR(10)  NOT NULL,
    decimal_places SMALLINT     NOT NULL DEFAULT 2 CHECK (decimal_places >= 0),
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE timezones (
    id         VARCHAR(64)  PRIMARY KEY,             -- "America/New_York"
    name       VARCHAR(100) NOT NULL,
    utc_offset VARCHAR(6)   NOT NULL,                -- "+05:30", "-05:00"
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
