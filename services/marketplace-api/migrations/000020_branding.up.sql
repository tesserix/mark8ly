CREATE TABLE store_branding (
    id                  UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID          NOT NULL,
    store_id            UUID          NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    -- Identity
    logo_url            TEXT,
    favicon_url         TEXT,
    tagline             VARCHAR(200),
    -- Colors (hex values)
    color_background    VARCHAR(7)    NOT NULL DEFAULT '#F7F6F2',
    color_text          VARCHAR(7)    NOT NULL DEFAULT '#0E0E0C',
    color_accent        VARCHAR(7)    NOT NULL DEFAULT '#2D4A2B',
    color_button_bg     VARCHAR(7)    NOT NULL DEFAULT '#0E0E0C',
    color_button_text   VARCHAR(7)    NOT NULL DEFAULT '#F7F6F2',
    -- Typography
    heading_font        VARCHAR(50)   NOT NULL DEFAULT 'source-serif-4',
    body_font           VARCHAR(50)   NOT NULL DEFAULT 'source-sans-3',
    -- Homepage
    layout_variant      VARCHAR(30)   NOT NULL DEFAULT 'classic-shop',
    hero_image_url      TEXT,
    announcement_text   VARCHAR(300),
    announcement_link   TEXT,
    announcement_bg     VARCHAR(7),
    announcement_active BOOLEAN       NOT NULL DEFAULT false,
    -- Footer
    footer_tagline      VARCHAR(300),
    footer_copyright    VARCHAR(200),
    social_instagram    VARCHAR(300),
    social_twitter      VARCHAR(300),
    social_facebook     VARCHAR(300),
    social_tiktok       VARCHAR(300),
    social_youtube      VARCHAR(300),
    -- Advanced (Enterprise)
    custom_css          TEXT,
    show_powered_by     BOOLEAN       NOT NULL DEFAULT true,
    -- Timestamps
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    UNIQUE (store_id)
);
CREATE INDEX sb_tenant_idx ON store_branding (tenant_id);
