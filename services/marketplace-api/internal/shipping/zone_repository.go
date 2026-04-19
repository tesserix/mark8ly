package shipping

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Zone is the default carrier + service mapping for an ISO 3166-1 alpha-2
// country code. Seeded by migration 000074 for IE/NZ/VN. Read-only from
// marketplace-api for now; admin UI writes land in a later phase.
type Zone struct {
	CountryCode        string    `gorm:"column:country_code;primaryKey"`
	CarrierID          string    `gorm:"column:carrier_id;not null"`
	DefaultServiceCode string    `gorm:"column:default_service_code"`
	Currency           string    `gorm:"column:currency"`
	CreatedAt          time.Time `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt          time.Time `gorm:"column:updated_at;not null;default:now()"`
}

// TableName maps Zone to the shipping_zones table. GORM's default
// pluralization would produce "zones"; this keeps us aligned with the
// migration-declared name.
func (Zone) TableName() string { return "shipping_zones" }

// ErrZoneNotFound is returned when no row matches the requested country.
var ErrZoneNotFound = errors.New("shipping: zone not found for country")

// ZoneRepository reads default carrier assignments per country.
type ZoneRepository struct {
	db *gorm.DB
}

// NewZoneRepository constructs a repository backed by the supplied *gorm.DB.
func NewZoneRepository(db *gorm.DB) *ZoneRepository {
	return &ZoneRepository{db: db}
}

// GetByCountry fetches the default carrier + service for the supplied ISO
// 3166-1 alpha-2 code. Country code is matched case-insensitively against
// the uppercased PRIMARY KEY value.
func (r *ZoneRepository) GetByCountry(ctx context.Context, countryCode string) (Zone, error) {
	cc := strings.ToUpper(strings.TrimSpace(countryCode))
	if cc == "" {
		return Zone{}, fmt.Errorf("shipping: GetByCountry: countryCode is required")
	}

	var z Zone
	err := r.db.WithContext(ctx).
		Where("country_code = ?", cc).
		First(&z).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Zone{}, fmt.Errorf("%w: %s", ErrZoneNotFound, cc)
	}
	if err != nil {
		return Zone{}, fmt.Errorf("shipping: GetByCountry(%s): %w", cc, err)
	}
	return z, nil
}
