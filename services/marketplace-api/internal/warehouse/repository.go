// Package warehouse owns the store-level pickup location (#177, the cheap
// half).
//
// # What it replaces
//
// A "warehouse" used to be eight `warehouse_*` columns hanging off
// `shipping_carrier_configs` — a row per (store, carrier). A merchant
// running Delhivery AND CouriersPlease typed the same physical address
// twice, into two rows, and kept them in sync by hand. The address is a
// property of the STORE, not of the carrier account used to ship from it.
//
// Migration 000095 did the expand half: created this table, backfilled one
// row per distinct (store, name), and added
// `shipping_carrier_configs.warehouse_id`. The old columns are still
// present and still written, deliberately — dropping them is the contract
// half, and only safe once every reader goes through warehouse_id.
//
// # What it deliberately does NOT do
//
// Multi-warehouse ALLOCATION. #177 argues, correctly, that choosing which
// warehouse ships an order is a product decision — nearest pincode, most
// stock, explicit priority? and what happens when no single location can
// fill an order — and that building it before a merchant with two
// warehouses asks means guessing their fulfilment rules.
//
// So this supports ONE default warehouse per store, which is exactly what
// every store has today. The table's shape and `variant_stock`'s
// (variant_id, location_id) key leave the door open; nothing here forecloses
// it.
package warehouse

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrNotFound signals the store has no warehouse.
//
// Distinct from a zero-value Warehouse on purpose: a caller that filled a
// carrier config with blank address fields would ship nothing, and would
// find out at the carrier API rather than here.
var ErrNotFound = errors.New("warehouse: not found")

// Warehouse is a store's pickup location.
type Warehouse struct {
	ID       string `gorm:"column:id;type:uuid;primaryKey"`
	TenantID string `gorm:"column:tenant_id;type:uuid;not null"`
	StoreID  string `gorm:"column:store_id;type:uuid;not null"`
	Name     string `gorm:"column:name;type:varchar(200);not null"`

	Line1      string `gorm:"column:line1;type:varchar(300);not null"`
	Line2      string `gorm:"column:line2;type:varchar(300);not null"`
	City       string `gorm:"column:city;type:varchar(200);not null"`
	Region     string `gorm:"column:region;type:varchar(200);not null"`
	PostalCode string `gorm:"column:postal_code;type:varchar(40);not null"`
	// CountryCode is ISO 3166-1 alpha-2.
	CountryCode string `gorm:"column:country_code;type:char(2);not null"`
	Phone       string `gorm:"column:phone;type:varchar(40);not null"`
	// Email and ContactPerson are nullable in migration 000095 and left
	// NULL by its backfill — the admin UI does not collect them yet. GORM
	// reads a NULL into the zero value, so a backfilled row is still safe
	// to load (pinned by a test).
	Email string `gorm:"column:email;type:varchar(200)"`
	// ContactPerson is required by Delhivery's warehouse registration and
	// had no home on the old carrier columns at all — which is how a label
	// failure ("ClientWarehouse matching query does not exist") led to
	// #177 in the first place.
	ContactPerson string `gorm:"column:contact_person;type:varchar(200)"`

	IsDefault bool      `gorm:"column:is_default;not null"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (Warehouse) TableName() string { return "warehouses" }

type Repository struct{}

func NewRepository() *Repository { return &Repository{} }

// Upsert creates the store's warehouse or updates it in place, keyed on
// (store_id, name) — the table's own unique index.
//
// Keying on the NAME rather than minting a row per call is the whole point:
// the second carrier configured for a store lands on the same row, so an
// edited address is seen by both instead of drifting between two copies.
//
// Runs in the caller's *gorm.DB so it can join a transaction that also
// writes the carrier config; a warehouse created for a config that then
// failed to save would be a row nothing references.
func (r *Repository) Upsert(ctx context.Context, db *gorm.DB, w Warehouse) (Warehouse, error) {
	w.Name = strings.TrimSpace(w.Name)
	if w.Name == "" {
		return Warehouse{}, fmt.Errorf("warehouse: name is required")
	}
	if w.StoreID == "" || w.TenantID == "" {
		return Warehouse{}, fmt.Errorf("warehouse: tenant and store are required")
	}
	w.CountryCode = strings.ToUpper(strings.TrimSpace(w.CountryCode))
	if w.ID == "" {
		// The column has a gen_random_uuid() default, but GORM sends the
		// zero value for a non-pointer string primary key rather than
		// omitting it, so Postgres receives "" and rejects it as a uuid.
		w.ID = uuid.NewString()
	}

	err := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "store_id"}, {Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"line1", "line2", "city", "region", "postal_code",
			"country_code", "phone", "email", "contact_person", "updated_at",
		}),
	}).Create(&w).Error
	if err != nil {
		return Warehouse{}, fmt.Errorf("warehouse: upsert: %w", err)
	}

	// Read back rather than trusting the in-memory struct: on the conflict
	// path the row's id is the EXISTING one, not the id GORM generated for
	// this call, and callers use it as a foreign key.
	return r.byStoreAndName(ctx, db, w.StoreID, w.Name)
}

// DefaultForStore returns the store's warehouse.
//
// Prefers the row flagged is_default, then the oldest — a deterministic
// answer matters because callers use it to fill a carrier config, and a
// result that varied per request would produce configs that disagree.
func (r *Repository) DefaultForStore(ctx context.Context, db *gorm.DB, storeID string) (Warehouse, error) {
	var w Warehouse
	err := db.WithContext(ctx).
		Where("store_id = ?", storeID).
		Order("is_default DESC, created_at ASC").
		First(&w).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Warehouse{}, ErrNotFound
	}
	if err != nil {
		return Warehouse{}, fmt.Errorf("warehouse: default for store: %w", err)
	}
	return w, nil
}

// ByID loads a warehouse by its primary key. This is the read side of
// #177's expand/contract migration: every site that used to read the
// pickup address off shipping_carrier_configs.warehouse_* now resolves it
// via a config's warehouse_id through this method instead, falling back to
// the legacy columns when warehouse_id is NULL or (rarely — the FK is ON
// DELETE SET NULL) points at a row that no longer exists.
func (r *Repository) ByID(ctx context.Context, db *gorm.DB, id string) (Warehouse, error) {
	var w Warehouse
	err := db.WithContext(ctx).Where("id = ?", id).First(&w).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Warehouse{}, ErrNotFound
	}
	if err != nil {
		return Warehouse{}, fmt.Errorf("warehouse: by id: %w", err)
	}
	return w, nil
}

func (r *Repository) byStoreAndName(ctx context.Context, db *gorm.DB, storeID, name string) (Warehouse, error) {
	var w Warehouse
	err := db.WithContext(ctx).
		Where("store_id = ? AND name = ?", storeID, name).First(&w).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Warehouse{}, ErrNotFound
	}
	if err != nil {
		return Warehouse{}, fmt.Errorf("warehouse: lookup: %w", err)
	}
	return w, nil
}
