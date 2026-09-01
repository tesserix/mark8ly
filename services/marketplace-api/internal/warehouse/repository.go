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

// ErrHasStock refuses deletion of a warehouse still holding inventory:
// deleting it would silently drop the units it holds from the store's
// availability with no record that they ever existed.
var ErrHasStock = errors.New("warehouse: still holds stock")

// ErrHasUnshippedParcel refuses deletion of a warehouse that owes a parcel.
// The allocation exists and is unshipped; removing its origin leaves an
// order that can never be fulfilled.
var ErrHasUnshippedParcel = errors.New("warehouse: has unshipped allocations")

// ErrHasHistory means the warehouse has allocation history and therefore
// cannot be deleted at all — it must be archived. Not a failure the caller
// should surface as an error: it is the signal to call Archive instead.
var ErrHasHistory = errors.New("warehouse: has allocation history; archive instead")

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

	IsDefault bool `gorm:"column:is_default;not null"`
	// Priority orders the allocator's candidate warehouses (000118).
	Priority int `gorm:"column:priority;not null"`
	// ArchivedAt marks a warehouse removed. Non-nil rows are excluded from
	// every list, from the allocator's candidates, and from the admin
	// pickers — but the row stays, because order_allocations.warehouse_id
	// is ON DELETE RESTRICT and the allocation is the record of which
	// warehouse shipped a line. Archiving IS removal for anything with
	// history; see Delete vs Archive below.
	ArchivedAt *time.Time `gorm:"column:archived_at"`
	CreatedAt  time.Time  `gorm:"column:created_at"`
	UpdatedAt  time.Time  `gorm:"column:updated_at"`
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
		// The unique index is PARTIAL (000122: WHERE archived_at IS NULL),
		// and Postgres will not infer a partial index from a bare column
		// list — it answers "no unique or exclusion constraint matching the
		// ON CONFLICT specification". The predicate has to be restated here
		// or every warehouse write fails, which is every carrier-config save.
		TargetWhere: clause.Where{
			Exprs: []clause.Expression{clause.Expr{SQL: "archived_at IS NULL"}},
		},
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
	return r.ByStoreAndName(ctx, db, w.StoreID, w.Name)
}

// DefaultForStore returns the store's warehouse.
//
// Prefers the row flagged is_default, then the oldest — a deterministic
// answer matters because callers use it to fill a carrier config, and a
// result that varied per request would produce configs that disagree.
func (r *Repository) DefaultForStore(ctx context.Context, db *gorm.DB, storeID string) (Warehouse, error) {
	var w Warehouse
	err := db.WithContext(ctx).
		// Archive() clears is_default, but that alone is not enough: with
		// every candidate then at is_default = false, created_at ASC would
		// hand back the archived row whenever it is the oldest, and a store
		// whose warehouses are ALL archived would get one rather than
		// ErrNotFound. Callers fill a carrier config's pickup address from
		// this, so an archived answer binds a live carrier to a warehouse
		// the allocator refuses to use.
		Where("store_id = ? AND archived_at IS NULL", storeID).
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

// ByStoreAndName loads a warehouse by the same key Upsert conflicts on
// (store_id, name) — the table's unique index. Exported so callers that
// need to resolve "the warehouse a given name currently maps to" can use
// the identical key Upsert itself uses, rather than going through a
// config's warehouse_id, which can go stale relative to that key: clearing
// warehouse_name on a config nils its warehouse_id (see settings.go's
// Upsert) while the underlying warehouses row survives untouched, so a
// later re-save of the SAME name must find that same row again even
// though nothing currently points at it by id.
func (r *Repository) ByStoreAndName(ctx context.Context, db *gorm.DB, storeID, name string) (Warehouse, error) {
	var w Warehouse
	// Archived rows are excluded deliberately. Since 000122 a store may
	// hold BOTH an archived and a live warehouse under one name (that is
	// what the partial index allows), and this is Upsert's read-back — so
	// matching an archived row would hand the caller a warehouse nothing
	// is allowed to allocate to, and point the carrier config at it.
	err := db.WithContext(ctx).
		Where("store_id = ? AND name = ? AND archived_at IS NULL", storeID, name).
		First(&w).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Warehouse{}, ErrNotFound
	}
	if err != nil {
		return Warehouse{}, fmt.Errorf("warehouse: lookup: %w", err)
	}
	return w, nil
}

// List returns a store's warehouses, most preferred first.
//
// Ordered by priority then name: priority is the allocator's own ordering
// (000118), and name breaks ties so two warehouses at the same priority do
// not swap places between calls. An unstable list would make the admin
// reorder UI jump under the merchant's cursor.
//
// Archived warehouses are excluded unless includeArchived. They must never
// reach the allocator or a picker; the flag exists for an audit view.
func (r *Repository) List(ctx context.Context, db *gorm.DB, storeID string, includeArchived bool) ([]Warehouse, error) {
	q := db.WithContext(ctx).Where("store_id = ?", storeID)
	if !includeArchived {
		q = q.Where("archived_at IS NULL")
	}
	var out []Warehouse
	if err := q.Order("priority ASC, name ASC").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("warehouse: list: %w", err)
	}
	return out, nil
}

// Archive marks a warehouse removed without deleting the row.
//
// This is what "remove" means for any warehouse with allocation history:
// order_allocations.warehouse_id is ON DELETE RESTRICT precisely so the
// record of which warehouse shipped a line cannot be destroyed.
//
// Idempotent — archiving an already-archived warehouse is a no-op rather
// than an error, so a double-click cannot rewrite the archive timestamp.
func (r *Repository) Archive(ctx context.Context, db *gorm.DB, id string) error {
	res := db.WithContext(ctx).
		Model(&Warehouse{}).
		Where("id = ? AND archived_at IS NULL", id).
		Updates(map[string]any{
			"archived_at": time.Now().UTC(),
			"updated_at":  time.Now().UTC(),
			// An archived warehouse must not remain the store's default:
			// DefaultForStore would keep handing out a warehouse nothing
			// is allowed to allocate to.
			"is_default": false,
		})
	if res.Error != nil {
		return fmt.Errorf("warehouse: archive: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		// Either it does not exist or it is already archived. Distinguish,
		// so a caller deleting a genuinely missing id still gets ErrNotFound.
		var count int64
		if err := db.WithContext(ctx).Model(&Warehouse{}).
			Where("id = ?", id).Count(&count).Error; err != nil {
			return fmt.Errorf("warehouse: archive: %w", err)
		}
		if count == 0 {
			return ErrNotFound
		}
	}
	return nil
}

// Delete removes a warehouse outright. Only legal for one with NO allocation
// history at all — anything else must be archived, and the FK's RESTRICT is
// the hard backstop if this check is ever bypassed.
//
// Refusals, in the order a merchant would want to hear them:
//
//	ErrHasStock            — still holds inventory
//	ErrHasUnshippedParcel  — owes a parcel that has not shipped
//	ErrHasHistory          — shipped in the past; archive instead
//
// Runs its checks and the delete in the caller's db so the whole thing can
// be one transaction: a check that passes and a delete that races another
// allocation would otherwise leave the FK to reject it with a raw error.
func (r *Repository) Delete(ctx context.Context, db *gorm.DB, id string) error {
	var stock int64
	if err := db.WithContext(ctx).
		Table("variant_stock").
		Where("location_id = ? AND quantity > 0", id).
		Count(&stock).Error; err != nil {
		return fmt.Errorf("warehouse: delete: stock check: %w", err)
	}
	if stock > 0 {
		return ErrHasStock
	}

	// Unshipped first: it is the more actionable of the two, and an
	// unshipped allocation is also history, so checking history first
	// would mask it behind the vaguer "archive instead".
	var unshipped int64
	if err := db.WithContext(ctx).
		Table("order_allocations").
		Where("warehouse_id = ? AND shipment_id IS NULL", id).
		Count(&unshipped).Error; err != nil {
		return fmt.Errorf("warehouse: delete: unshipped check: %w", err)
	}
	if unshipped > 0 {
		return ErrHasUnshippedParcel
	}

	var history int64
	if err := db.WithContext(ctx).
		Table("order_allocations").
		Where("warehouse_id = ?", id).
		Count(&history).Error; err != nil {
		return fmt.Errorf("warehouse: delete: history check: %w", err)
	}
	if history > 0 {
		return ErrHasHistory
	}

	res := db.WithContext(ctx).Where("id = ?", id).Delete(&Warehouse{})
	if res.Error != nil {
		return fmt.Errorf("warehouse: delete: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// PriorityUpdate is one entry in a reorder.
type PriorityUpdate struct {
	ID       string
	Priority int
}

// SetPriorities rewrites the ordering of a store's warehouses in ONE
// transaction.
//
// Takes the full ordered set rather than a delta: a delta applied to a list
// that changed underneath (a warehouse archived in another tab) reorders the
// wrong rows. A partial application is not acceptable either — half-applied
// priorities are a silently different allocation order, so any failure rolls
// the whole thing back.
//
// Scoped by store_id as well as id so a crafted request cannot reprioritise
// another store's warehouses.
func (r *Repository) SetPriorities(ctx context.Context, db *gorm.DB, storeID string, updates []PriorityUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, u := range updates {
			res := tx.Model(&Warehouse{}).
				Where("id = ? AND store_id = ? AND archived_at IS NULL", u.ID, storeID).
				Updates(map[string]any{
					"priority":   u.Priority,
					"updated_at": time.Now().UTC(),
				})
			if res.Error != nil {
				return fmt.Errorf("warehouse: set priorities: %w", res.Error)
			}
			if res.RowsAffected == 0 {
				// An id that is not this store's, or is archived. Failing
				// the whole reorder is deliberate: applying the rest would
				// leave an ordering the merchant never asked for.
				return fmt.Errorf("warehouse: set priorities: %w (id %s)", ErrNotFound, u.ID)
			}
		}
		return nil
	})
}
