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
	ID                 uuid.UUID       `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	TenantID           uuid.UUID       `gorm:"column:tenant_id;type:uuid;not null;index"`
	StoreID            uuid.UUID       `gorm:"column:store_id;type:uuid;not null;index"`
	OrderID            uuid.UUID       `gorm:"column:order_id;type:uuid;not null;index"`
	Carrier            string          `gorm:"column:carrier;type:varchar(20);not null"`
	TrackingNumber     string          `gorm:"column:tracking_number;type:varchar(100)"`
	LabelURL           string          `gorm:"column:label_url;type:text"`
	Status             string          `gorm:"column:status;type:varchar(20);not null;default:pending"`
	ShipFrom           datatypes.JSON  `gorm:"column:ship_from;type:jsonb;not null"`
	ShipTo             datatypes.JSON  `gorm:"column:ship_to;type:jsonb;not null"`
	HandlingFee        decimal.Decimal `gorm:"column:handling_fee;type:numeric(12,2);not null;default:0"`
	TotalCost          decimal.Decimal `gorm:"column:total_cost;type:numeric(12,2)"`
	CurrencyCode       string          `gorm:"column:currency_code;type:char(3);not null"`
	EstimatedDelivery  *time.Time      `gorm:"column:estimated_delivery"`
	CreatedAt          time.Time       `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt          time.Time       `gorm:"column:updated_at;not null;default:now()"`

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
	Enabled               bool            `gorm:"column:is_active;not null;default:false"`
	WarehouseName         string          `gorm:"column:warehouse_name;type:varchar(200)"`
	WarehouseLine1        string          `gorm:"column:warehouse_line1;type:varchar(300)"`
	WarehouseLine2        string          `gorm:"column:warehouse_line2;type:varchar(300)"`
	WarehouseCity         string          `gorm:"column:warehouse_city;type:varchar(200)"`
	WarehouseRegion       string          `gorm:"column:warehouse_region;type:varchar(200)"`
	WarehousePostal       string          `gorm:"column:warehouse_postal;type:varchar(40)"`
	WarehouseCountry      string          `gorm:"column:warehouse_country;type:char(2)"`
	WarehousePhone        string          `gorm:"column:warehouse_phone;type:varchar(40)"`
	CreatedAt             time.Time       `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt             time.Time       `gorm:"column:updated_at;not null;default:now()"`
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
	ListShipmentsByStore(ctx context.Context, storeID uuid.UUID, limit, offset int) ([]ShipmentRecord, int64, error)
	UpdateShipmentStatus(ctx context.Context, id uuid.UUID, status string) error

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

func (r *gormRepository) GetShipmentByOrderID(ctx context.Context, orderID uuid.UUID) (*ShipmentRecord, error) {
	var rec ShipmentRecord
	err := r.db.WithContext(ctx).Where("order_id = ?", orderID).First(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("shipping: shipment not found for order")
	}
	if err != nil {
		return nil, fmt.Errorf("shipping: get shipment by order id: %w", err)
	}
	return &rec, nil
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
			WarehouseName:         cfg.WarehouseName,
			WarehouseLine1:        cfg.WarehouseLine1,
			WarehouseLine2:        cfg.WarehouseLine2,
			WarehouseCity:         cfg.WarehouseCity,
			WarehouseRegion:       cfg.WarehouseRegion,
			WarehousePostal:       cfg.WarehousePostal,
			WarehouseCountry:      cfg.WarehouseCountry,
			WarehousePhone:        cfg.WarehousePhone,
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
