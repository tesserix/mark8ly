package main

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/carriersecrets"
)

// Row is one migratable credential slot: a single (table, column) cell
// whose value carriersecrets.Scope maps 1:1 to one secret. Multiple Rows
// can share the same ID (a payment_gateway_configs row with all three of
// api_key/secret_key/webhook_secret populated becomes three Rows).
type Row struct {
	// Table and Column name the DB cell this Row was read from and will be
	// written back to. Both are always one of the fixed literals this
	// package produces — never derived from external input — so passing
	// Column into a dynamic gorm Update is safe.
	Table  string
	Column string
	// ID is the row's primary key, as its canonical string form.
	ID string
	// Scope is the (tenant, domain, provider, field) tuple this cell
	// resolves to — see the per-table *Rows functions below for exactly
	// how each table's columns map onto it.
	Scope carriersecrets.Scope
	// Ref is the cell's current value: "", a bao:// or gsm:// reference,
	// or legacy inline ciphertext. The backfill only acts on gsm:// values
	// — see Backfiller.Run.
	Ref string
}

// RowStore is the DB access surface the backfill needs. Production wires
// gormRowStore against the real schema; tests use a recording fake so
// dry-run and verification-failure behaviour can be asserted without a
// database.
type RowStore interface {
	// FetchAll returns every migratable credential cell across all four
	// tracked tables, regardless of what Ref currently holds — the
	// gsm://-only filter is Backfiller.Run's job, not the fetch's, so
	// unit tests can feed it a mix of bao:///inline/empty/gsm:// rows and
	// assert on the skip behaviour directly.
	FetchAll(ctx context.Context) ([]Row, error)
	// UpdateReference persists newRef into row's own (table, column, id).
	// Callers MUST have already verified newRef round-trips to the same
	// plaintext as row.Ref before calling this — see Backfiller.migrateRow.
	UpdateReference(ctx context.Context, row Row, newRef string) error
}

// gormRowStore implements RowStore against the live marketplace-api schema.
type gormRowStore struct {
	db *gorm.DB
}

func newGormRowStore(db *gorm.DB) *gormRowStore { return &gormRowStore{db: db} }

// ─────────────────────────────────────────────────────────────────────────
// payment_gateway_configs — internal/handlers/admin/settings.go:551-558
// writes api_key/secret_key/webhook_secret under Domain "payment" with the
// row's own `provider` column as Scope.Provider.
// ─────────────────────────────────────────────────────────────────────────

type paymentGatewayConfigRow struct {
	ID                     uuid.UUID `gorm:"column:id"`
	TenantID               uuid.UUID `gorm:"column:tenant_id"`
	Provider               string    `gorm:"column:provider"`
	APIKeyEncrypted        string    `gorm:"column:api_key_encrypted"`
	SecretKeyEncrypted     string    `gorm:"column:secret_key_encrypted"`
	WebhookSecretEncrypted string    `gorm:"column:webhook_secret_encrypted"`
}

func (paymentGatewayConfigRow) TableName() string { return "payment_gateway_configs" }

// paymentRows derives the three migratable cells of one
// payment_gateway_configs row. Kept as a pure function (no DB, no Store) so
// the scope mapping can be unit-tested directly against BaoPath.
func paymentRows(r paymentGatewayConfigRow) []Row {
	id := r.ID.String()
	tenant := r.TenantID.String()
	scope := func(field string) carriersecrets.Scope {
		return carriersecrets.Scope{TenantID: tenant, Domain: "payment", Provider: r.Provider, Field: field}
	}
	return []Row{
		{Table: "payment_gateway_configs", Column: "api_key_encrypted", ID: id, Ref: r.APIKeyEncrypted, Scope: scope("api_key")},
		{Table: "payment_gateway_configs", Column: "secret_key_encrypted", ID: id, Ref: r.SecretKeyEncrypted, Scope: scope("secret_key")},
		{Table: "payment_gateway_configs", Column: "webhook_secret_encrypted", ID: id, Ref: r.WebhookSecretEncrypted, Scope: scope("webhook_secret")},
	}
}

// ─────────────────────────────────────────────────────────────────────────
// shipping_carrier_configs — internal/handlers/admin/settings.go:~718-725
// (ShippingSettingsHandler.putCredential) writes api_key/secret_key under
// Domain "shipping" with the row's own `provider` column (NOT `carrier` —
// confirmed against internal/shipping/repository.go's CarrierConfig and
// internal/handlers/storefront/shipping_rates.go's carrierConfigRow, both
// of which tag the same table's provider column as `provider`).
// ─────────────────────────────────────────────────────────────────────────

type shippingCarrierConfigRow struct {
	ID                 uuid.UUID `gorm:"column:id"`
	TenantID           uuid.UUID `gorm:"column:tenant_id"`
	Provider           string    `gorm:"column:provider"`
	APIKeyEncrypted    string    `gorm:"column:api_key_encrypted"`
	SecretKeyEncrypted string    `gorm:"column:secret_key_encrypted"`
}

func (shippingCarrierConfigRow) TableName() string { return "shipping_carrier_configs" }

func shippingRows(r shippingCarrierConfigRow) []Row {
	id := r.ID.String()
	tenant := r.TenantID.String()
	scope := func(field string) carriersecrets.Scope {
		return carriersecrets.Scope{TenantID: tenant, Domain: "shipping", Provider: r.Provider, Field: field}
	}
	return []Row{
		{Table: "shipping_carrier_configs", Column: "api_key_encrypted", ID: id, Ref: r.APIKeyEncrypted, Scope: scope("api_key")},
		{Table: "shipping_carrier_configs", Column: "secret_key_encrypted", ID: id, Ref: r.SecretKeyEncrypted, Scope: scope("secret_key")},
	}
}

// ─────────────────────────────────────────────────────────────────────────
// tax_provider_configs — internal/handlers/admin/settings.go:~1440-1447
// (TaxSettingsHandler.putCredential) writes api_key under Domain "tax" with
// the row's own `provider` column.
// ─────────────────────────────────────────────────────────────────────────

type taxProviderConfigRow struct {
	ID              uuid.UUID `gorm:"column:id"`
	TenantID        uuid.UUID `gorm:"column:tenant_id"`
	Provider        string    `gorm:"column:provider"`
	APIKeyEncrypted string    `gorm:"column:api_key_encrypted"`
}

func (taxProviderConfigRow) TableName() string { return "tax_provider_configs" }

func taxRows(r taxProviderConfigRow) []Row {
	id := r.ID.String()
	tenant := r.TenantID.String()
	return []Row{
		{
			Table: "tax_provider_configs", Column: "api_key_encrypted", ID: id, Ref: r.APIKeyEncrypted,
			Scope: carriersecrets.Scope{TenantID: tenant, Domain: "tax", Provider: r.Provider, Field: "api_key"},
		},
	}
}

// ─────────────────────────────────────────────────────────────────────────
// custom_domains — internal/domain/service.go:150-155 (scopeForCFToken)
// writes cf_api_token_encrypted under Domain "platform", Provider fixed to
// "cloudflare", and Field set to the row's own FQDN (the `domain` column) —
// NOT a fixed field name, since a tenant can register multiple custom
// domains and each needs its own secret slot.
// ─────────────────────────────────────────────────────────────────────────

type customDomainRow struct {
	ID                  uuid.UUID `gorm:"column:id"`
	TenantID            uuid.UUID `gorm:"column:tenant_id"`
	Domain              string    `gorm:"column:domain"`
	CFAPITokenEncrypted string    `gorm:"column:cf_api_token_encrypted"`
}

func (customDomainRow) TableName() string { return "custom_domains" }

func domainRows(r customDomainRow) []Row {
	return []Row{
		{
			Table: "custom_domains", Column: "cf_api_token_encrypted", ID: r.ID.String(), Ref: r.CFAPITokenEncrypted,
			Scope: carriersecrets.Scope{TenantID: r.TenantID.String(), Domain: "platform", Provider: "cloudflare", Field: r.Domain},
		},
	}
}

// FetchAll queries all four tracked tables and flattens every row into its
// migratable cells.
func (g *gormRowStore) FetchAll(ctx context.Context) ([]Row, error) {
	var out []Row

	var payments []paymentGatewayConfigRow
	if err := g.db.WithContext(ctx).Find(&payments).Error; err != nil {
		return nil, fmt.Errorf("query payment_gateway_configs: %w", err)
	}
	for _, r := range payments {
		out = append(out, paymentRows(r)...)
	}

	var shipping []shippingCarrierConfigRow
	if err := g.db.WithContext(ctx).Find(&shipping).Error; err != nil {
		return nil, fmt.Errorf("query shipping_carrier_configs: %w", err)
	}
	for _, r := range shipping {
		out = append(out, shippingRows(r)...)
	}

	var tax []taxProviderConfigRow
	if err := g.db.WithContext(ctx).Find(&tax).Error; err != nil {
		return nil, fmt.Errorf("query tax_provider_configs: %w", err)
	}
	for _, r := range tax {
		out = append(out, taxRows(r)...)
	}

	var domains []customDomainRow
	if err := g.db.WithContext(ctx).Find(&domains).Error; err != nil {
		return nil, fmt.Errorf("query custom_domains: %w", err)
	}
	for _, r := range domains {
		out = append(out, domainRows(r)...)
	}

	return out, nil
}

// UpdateReference writes newRef into row's own cell. Column is always one
// of this package's fixed literals (never external input), so passing it
// into gorm's dynamic Update is safe.
func (g *gormRowStore) UpdateReference(ctx context.Context, row Row, newRef string) error {
	res := g.db.WithContext(ctx).Table(row.Table).Where("id = ?", row.ID).Update(row.Column, newRef)
	if res.Error != nil {
		return fmt.Errorf("update %s.%s for id %s: %w", row.Table, row.Column, row.ID, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("update %s.%s for id %s: no rows affected (row deleted since it was fetched?)", row.Table, row.Column, row.ID)
	}
	return nil
}
