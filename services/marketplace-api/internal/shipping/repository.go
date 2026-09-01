package shipping

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ──────────────────────────────────────────────────────────────────────
// GORM models
// ──────────────────────────────────────────────────────────────────────

// ShipmentRecord is the GORM model for the shipments table. Column names
// match the migration 000008 schema — earlier drafts of this model drifted
// (provider → carrier, provider_shipment_id column that doesn't exist,
// missing ship_from/ship_to JSONB-not-null), which made every INSERT hit
// "column does not exist" / "null value" and the admin saw a generic
// "internal server error" when trying to generate a shipping label.
//
// The Provider / ProviderShipmentID / Service fields are kept in the Go
// struct for the handler's response shape, but they're explicitly marked
// `gorm:"-"` so the ORM never sends them to the DB.
type ShipmentRecord struct {
	ID                uuid.UUID       `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID          uuid.UUID       `gorm:"column:tenant_id;type:uuid;not null;index"`
	StoreID           uuid.UUID       `gorm:"column:store_id;type:uuid;not null;index"`
	OrderID           uuid.UUID       `gorm:"column:order_id;type:uuid;not null;index"`
	Carrier           string          `gorm:"column:carrier;type:varchar(20);not null"`
	TrackingNumber    string          `gorm:"column:tracking_number;type:varchar(100)"`
	LabelURL          string          `gorm:"column:label_url;type:text"`
	Status            string          `gorm:"column:status;type:varchar(20);not null;default:pending"`
	ShipFrom          datatypes.JSON  `gorm:"column:ship_from;type:jsonb;not null"`
	ShipTo            datatypes.JSON  `gorm:"column:ship_to;type:jsonb;not null"`
	HandlingFee       decimal.Decimal `gorm:"column:handling_fee;type:numeric(12,2);not null;default:0"`
	TotalCost         decimal.Decimal `gorm:"column:total_cost;type:numeric(12,2)"`
	CurrencyCode      string          `gorm:"column:currency_code;type:char(3);not null"`
	EstimatedDelivery *time.Time      `gorm:"column:estimated_delivery"`
	// ShippedAt + DeliveredAt are stamped by the admin status update
	// handler when the row transitions into the corresponding state.
	// They lived in the schema (migration 000008) for a long time but
	// weren't on the GORM model, so the values were unreadable from
	// Go and consumers had to hit the table directly. Surfacing them
	// here lets the receipt PDF stamp the real delivery moment instead
	// of falling back to order.updated_at as a proxy.
	ShippedAt   *time.Time `gorm:"column:shipped_at"`
	DeliveredAt *time.Time `gorm:"column:delivered_at"`
	CreatedAt   time.Time  `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt   time.Time  `gorm:"column:updated_at;not null;default:now()"`

	// Pickup scheduling (Delhivery). PickupRequestID carries either the
	// carrier's numeric pr_id (stringified) or the sentinel
	// "already-scheduled" when we detected a duplicate and persisted
	// anyway. Nullable TIMESTAMPTZ on the DB side so rows created
	// before the pickup-scheduling feature shipped stay valid.
	PickupRequestID    string     `gorm:"column:pickup_request_id;type:varchar(100)"`
	PickupScheduledFor *time.Time `gorm:"column:pickup_scheduled_for"`

	// Cancel/return action taken when the order was refunded or cancelled.
	// Written best-effort by internal/shipmentcancel; NOT touched on the
	// normal create/track path. Defaults keep pre-feature rows valid.
	CancelAction      string     `gorm:"column:cancel_action;type:varchar(20);not null;default:none"`
	CancelStatus      string     `gorm:"column:cancel_status;type:varchar(20);not null;default:none"`
	CancelReason      string     `gorm:"column:cancel_reason;type:text"`
	CancelRequestedAt *time.Time `gorm:"column:cancel_requested_at"`

	// WarehouseID is where this shipment actually shipped from (#177,
	// migration 000118). Nullable: every shipment created before that
	// migration has no honest answer, and a shipment created for an order
	// with no allocations (the whole store today) attributes to the
	// carrier config's warehouse instead of a specific allocation group,
	// so it is left NULL rather than guessed.
	WarehouseID *uuid.UUID `gorm:"column:warehouse_id;type:uuid"`

	// Response-shape only — never touch the DB. Callers map these after
	// reading a row, and the carrier layer sets them on create.
	Provider           string `gorm:"-"`
	ProviderShipmentID string `gorm:"-"`
	Service            string `gorm:"-"`
}

func (ShipmentRecord) TableName() string { return "shipments" }

// CarrierConfig is the GORM model for the shipping_carrier_configs table.
// Column names here intentionally match the migration 000008 schema —
// earlier drafts of this model drifted (api_key → api_key_encrypted,
// enabled → is_active, free_shipping_threshold → free_shipping_min),
// which made admin settings writes invisible to the shipments handler
// reader and surfaced as "no carrier config found for provider".
type CarrierConfig struct {
	ID                    uuid.UUID       `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID              uuid.UUID       `gorm:"column:tenant_id;type:uuid;not null;index"`
	StoreID               uuid.UUID       `gorm:"column:store_id;type:uuid;not null;index"`
	Provider              string          `gorm:"column:provider;type:varchar(40);not null"`
	APIKey                string          `gorm:"column:api_key_encrypted;type:text;not null"`
	SecretKey             string          `gorm:"column:secret_key_encrypted;type:text"`
	Mode                  string          `gorm:"column:mode;type:varchar(10);not null;default:test"`
	HandlingFee           decimal.Decimal `gorm:"column:handling_fee;type:numeric(12,2);not null;default:0"`
	FreeShippingThreshold decimal.Decimal `gorm:"column:free_shipping_min;type:numeric(12,2)"`
	// DefaultParcelWeightGrams is used when a product carries no weight.
	// Checkout hardcoded 500 for that case, so an invisible constant in
	// frontend code was setting real carrier prices. Migration 000120
	// defaults this to 500 so no live quote moves; it only makes the
	// number visible and adjustable. Per-product weights remain the
	// accurate answer — this is the fallback when one is missing.
	DefaultParcelWeightGrams int  `gorm:"column:default_parcel_weight_grams;not null;default:500"`
	Enabled                  bool `gorm:"column:is_active;not null;default:false"`
	// WarehouseID points at the store-level warehouses row (migration
	// 000095, #177) when one has been linked. Nullable: a config saved
	// with a blank warehouse name never gets one, and the FK is ON DELETE
	// SET NULL. *uuid.UUID rather than uuid.UUID so GORM writes/reads SQL
	// NULL instead of the zero UUID.
	//
	// #484 dropped the WarehouseName/Line1/Line2/City/Region/Postal/
	// Country/Phone fields that used to mirror this row's legacy
	// warehouse_* columns: nothing reads them anymore, which is what
	// makes dropping those columns in a later migration safe.
	WarehouseID *uuid.UUID `gorm:"column:warehouse_id;type:uuid"`
	// Pickup automation. AutoSchedulePickup is the master toggle; rows
	// default to TRUE in SQL so existing configs auto-opt-in when the
	// shipping_pickup.sql ALTERs apply. DefaultPickupSlotStart is fed
	// back to Delhivery verbatim as pickup_time; DefaultPickupSlotEnd
	// is UI-only ("Pickup: 14:00 – 18:00"). Keeping the slot columns
	// as VARCHAR(8) mirrors Delhivery's HH:MM:SS wire format — we
	// never do arithmetic on them server-side.
	AutoSchedulePickup     bool      `gorm:"column:auto_schedule_pickup;type:boolean;not null;default:true"`
	DefaultPickupSlotStart string    `gorm:"column:default_pickup_slot_start;type:varchar(8);default:14:00:00"`
	DefaultPickupSlotEnd   string    `gorm:"column:default_pickup_slot_end;type:varchar(8);default:18:00:00"`
	CreatedAt              time.Time `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt              time.Time `gorm:"column:updated_at;not null;default:now()"`
}

func (CarrierConfig) TableName() string { return "shipping_carrier_configs" }

// ──────────────────────────────────────────────────────────────────────
// Repository interface
// ──────────────────────────────────────────────────────────────────────

// Repository is the data-access surface for shipping persistence.
type Repository interface {
	// Shipments
	CreateShipment(ctx context.Context, rec *ShipmentRecord) error
	GetShipmentByID(ctx context.Context, id uuid.UUID) (*ShipmentRecord, error)
	GetShipmentByOrderID(ctx context.Context, orderID uuid.UUID) (*ShipmentRecord, error)
	// GetShipmentByTrackingNumber resolves a (carrier, AWB) tuple to
	// the local shipment record. Added for the Delhivery webhook
	// receiver, which has only the AWB in hand and must not leak
	// existence of other shipments — callers treat a
	// not-found the same way as a success.
	GetShipmentByTrackingNumber(ctx context.Context, carrier, trackingNumber string) (*ShipmentRecord, error)
	ListShipmentsByOrderID(ctx context.Context, orderID uuid.UUID) ([]ShipmentRecord, error)
	ListShipmentsByStore(ctx context.Context, storeID uuid.UUID, limit, offset int) ([]ShipmentRecord, int64, error)
	UpdateShipmentStatus(ctx context.Context, id uuid.UUID, status string) error
	SetShipmentCancelState(ctx context.Context, shipmentID uuid.UUID, action, status, reason string) error
	ReleaseAllocationsForShipment(ctx context.Context, shipmentID uuid.UUID) (int64, error)

	// Carrier configs
	GetCarrierConfig(ctx context.Context, storeID, provider string) (*CarrierConfig, error)
	ListCarrierConfigs(ctx context.Context, storeID string) ([]CarrierConfig, error)
	UpsertCarrierConfig(ctx context.Context, cfg *CarrierConfig) error
	DeleteCarrierConfig(ctx context.Context, id uuid.UUID) error
}

// ──────────────────────────────────────────────────────────────────────
// GORM implementation
// ──────────────────────────────────────────────────────────────────────

type gormRepository struct {
	db *gorm.DB
}

// NewRepository constructs a Repository backed by GORM.
func NewRepository(db *gorm.DB) Repository { return &gormRepository{db: db} }

// --- Shipments ---

func (r *gormRepository) CreateShipment(ctx context.Context, rec *ShipmentRecord) error {
	if rec.ID == uuid.Nil {
		rec.ID = uuid.New()
	}
	if err := r.db.WithContext(ctx).Create(rec).Error; err != nil {
		return fmt.Errorf("shipping: create shipment record: %w", err)
	}
	return nil
}

func (r *gormRepository) GetShipmentByID(ctx context.Context, id uuid.UUID) (*ShipmentRecord, error) {
	var rec ShipmentRecord
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("shipping: shipment not found")
	}
	if err != nil {
		return nil, fmt.Errorf("shipping: get shipment by id: %w", err)
	}
	return &rec, nil
}

// GetShipmentByOrderID returns the first parcel on an order, ordered
// created_at ASC, id ASC. A multi-warehouse order has one shipments row per
// warehouse (#177); without an explicit order Postgres returns whichever row
// it likes (effectively by primary key), which made this pick an arbitrary
// parcel. Ordering here matches storefront/order_detail.go's loadShipments
// (added in #492) so "the shipment" means the same parcel everywhere (#496).
func (r *gormRepository) GetShipmentByOrderID(ctx context.Context, orderID uuid.UUID) (*ShipmentRecord, error) {
	var rec ShipmentRecord
	err := r.db.WithContext(ctx).
		Where("order_id = ?", orderID).
		Order("created_at ASC, id ASC").
		First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("shipping: shipment not found for order")
	}
	if err != nil {
		return nil, fmt.Errorf("shipping: get shipment by order id: %w", err)
	}
	return &rec, nil
}

// GetShipmentByTrackingNumber returns the shipment whose carrier + AWB
// match the inputs. ErrRecordNotFound is wrapped in a domain-level
// error string so the webhook handler can distinguish a lookup miss
// (treat as success, don't leak existence) from a transient DB error
// (treat as auth-failed to avoid ack'ing a webhook we couldn't verify).
func (r *gormRepository) GetShipmentByTrackingNumber(ctx context.Context, carrier, trackingNumber string) (*ShipmentRecord, error) {
	var rec ShipmentRecord
	err := r.db.WithContext(ctx).
		Where("carrier = ? AND tracking_number = ?", carrier, trackingNumber).
		First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("shipping: shipment not found for tracking number")
	}
	if err != nil {
		return nil, fmt.Errorf("shipping: get shipment by tracking number: %w", err)
	}
	return &rec, nil
}

func (r *gormRepository) ListShipmentsByOrderID(ctx context.Context, orderID uuid.UUID) ([]ShipmentRecord, error) {
	var recs []ShipmentRecord
	if err := r.db.WithContext(ctx).
		Where("order_id = ?", orderID).
		Order("created_at ASC, id ASC").
		Find(&recs).Error; err != nil {
		return nil, fmt.Errorf("shipping: list shipments by order id: %w", err)
	}
	return recs, nil
}

// SetShipmentCancelState records the outcome of a carrier cancel/return
// attempt on a shipment. Stamps cancel_requested_at + updated_at to now().
func (r *gormRepository) SetShipmentCancelState(ctx context.Context, shipmentID uuid.UUID, action, status, reason string) error {
	res := r.db.WithContext(ctx).
		Table("shipments").
		Where("id = ?", shipmentID).
		Updates(map[string]any{
			"cancel_action":       action,
			"cancel_status":       status,
			"cancel_reason":       reason,
			"cancel_requested_at": time.Now().UTC(),
			"updated_at":          time.Now().UTC(),
		})
	if res.Error != nil {
		return fmt.Errorf("shipping: set shipment cancel state: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("shipping: shipment not found")
	}
	return nil
}

// ReleaseAllocationsForShipment clears order_allocations.shipment_id for
// every row stamped with shipmentID, making those allocation groups
// unshipped again so a new label can be created for them.
//
// Called ONLY after a cancel that means the goods never left
// (pending/created/manifested → ActionCancelForward). An in-transit or
// delivered shipment must keep its stamp: freeing those would let a
// merchant create a second label for goods already moving.
//
// Returns the number of allocations freed so the caller can log it. Zero
// is normal, not an error — orders placed before allocation shipped have
// no allocation rows at all.
func (r *gormRepository) ReleaseAllocationsForShipment(ctx context.Context, shipmentID uuid.UUID) (int64, error) {
	res := r.db.WithContext(ctx).
		Table("order_allocations").
		Where("shipment_id = ?", shipmentID).
		Updates(map[string]any{
			"shipment_id": nil,
			"updated_at":  time.Now().UTC(),
		})
	if res.Error != nil {
		return 0, fmt.Errorf("shipping: release allocations for shipment: %w", res.Error)
	}
	return res.RowsAffected, nil
}

func (r *gormRepository) ListShipmentsByStore(ctx context.Context, storeID uuid.UUID, limit, offset int) ([]ShipmentRecord, int64, error) {
	var total int64
	q := r.db.WithContext(ctx).Model(&ShipmentRecord{}).Where("store_id = ?", storeID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("shipping: list shipments count: %w", err)
	}

	var recs []ShipmentRecord
	err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&recs).Error
	if err != nil {
		return nil, 0, fmt.Errorf("shipping: list shipments: %w", err)
	}
	return recs, total, nil
}

func (r *gormRepository) UpdateShipmentStatus(ctx context.Context, id uuid.UUID, status string) error {
	res := r.db.WithContext(ctx).
		Model(&ShipmentRecord{}).
		Where("id = ?", id).
		Update("status", status)
	if res.Error != nil {
		return fmt.Errorf("shipping: update shipment status: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("shipping: shipment not found")
	}
	return nil
}

// --- Carrier configs ---

func (r *gormRepository) GetCarrierConfig(ctx context.Context, storeID, provider string) (*CarrierConfig, error) {
	var cfg CarrierConfig
	err := r.db.WithContext(ctx).
		Where("store_id = ? AND provider = ? AND is_active = true", storeID, provider).
		First(&cfg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("shipping: carrier config not found")
	}
	if err != nil {
		return nil, fmt.Errorf("shipping: get carrier config: %w", err)
	}
	return &cfg, nil
}

func (r *gormRepository) ListCarrierConfigs(ctx context.Context, storeID string) ([]CarrierConfig, error) {
	var cfgs []CarrierConfig
	err := r.db.WithContext(ctx).
		Where("store_id = ?", storeID).
		Order("provider ASC").
		Find(&cfgs).Error
	if err != nil {
		return nil, fmt.Errorf("shipping: list carrier configs: %w", err)
	}
	return cfgs, nil
}

func (r *gormRepository) UpsertCarrierConfig(ctx context.Context, cfg *CarrierConfig) error {
	if cfg.ID == uuid.Nil {
		cfg.ID = uuid.New()
	}

	var existing CarrierConfig
	err := r.db.WithContext(ctx).
		Where("store_id = ? AND provider = ?", cfg.StoreID, cfg.Provider).
		First(&existing).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// No existing row — create.
		if err := r.db.WithContext(ctx).Create(cfg).Error; err != nil {
			return fmt.Errorf("shipping: create carrier config: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("shipping: upsert carrier config: lookup: %w", err)
	}

	// Existing row found — update it.
	cfg.ID = existing.ID
	err = r.db.WithContext(ctx).
		Model(&existing).
		Updates(CarrierConfig{
			APIKey:                cfg.APIKey,
			SecretKey:             cfg.SecretKey,
			Mode:                  cfg.Mode,
			HandlingFee:           cfg.HandlingFee,
			FreeShippingThreshold: cfg.FreeShippingThreshold,
			Enabled:               cfg.Enabled,
			// #484: this used to write the 8 legacy warehouse_* columns
			// from cfg's now-removed fields. UpsertCarrierConfig has no
			// production caller (see the package doc on this method), so
			// there was no live divergence to fix — but if it's ever
			// wired up, it must maintain warehouse_id, the only field any
			// reader still looks at, or a caller pointing at a real
			// warehouse would silently write an id the read path then
			// ignores. Note GORM's Updates(struct) skips zero-value
			// fields, so a caller passing a nil WarehouseID here leaves
			// the stored value untouched rather than clearing it — unlike
			// the admin settings handler's map-based Updates, which does
			// clear it. That's acceptable for now given there is no
			// caller to observe the difference.
			WarehouseID: cfg.WarehouseID,
		}).Error
	if err != nil {
		return fmt.Errorf("shipping: update carrier config: %w", err)
	}
	return nil
}

func (r *gormRepository) DeleteCarrierConfig(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&CarrierConfig{})
	if res.Error != nil {
		return fmt.Errorf("shipping: delete carrier config: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("shipping: carrier config not found")
	}
	return nil
}
