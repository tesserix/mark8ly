package location

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	apperrors "github.com/mark8ly/platform-api/pkg/errors"
)

// Repository is the data-access interface for location reference data.
//
// Defined as an interface (not a concrete struct) so the service layer can be
// unit-tested with a fake. The integration tests run the real implementation
// against a real Postgres via pkg/testdb.
type Repository interface {
	ListCountries(ctx context.Context) ([]Country, error)
	GetCountry(ctx context.Context, code string) (*Country, error)
	ListStatesByCountry(ctx context.Context, countryCode string) ([]State, error)
	GetState(ctx context.Context, id string) (*State, error)
	ListCurrencies(ctx context.Context) ([]Currency, error)
	GetCurrency(ctx context.Context, code string) (*Currency, error)
	ListTimezones(ctx context.Context) ([]Timezone, error)
	GetTimezone(ctx context.Context, id string) (*Timezone, error)
}

// gormRepository is the GORM-backed implementation of Repository.
type gormRepository struct {
	db *gorm.DB
}

// NewRepository constructs a Repository backed by the given GORM connection.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) ListCountries(ctx context.Context) ([]Country, error) {
	var out []Country
	if err := r.db.WithContext(ctx).Order("name asc").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("location: list countries: %w", err)
	}
	return out, nil
}

func (r *gormRepository) GetCountry(ctx context.Context, code string) (*Country, error) {
	var c Country
	err := r.db.WithContext(ctx).Where("code = ?", code).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("country_not_found", fmt.Sprintf("country %q does not exist", code))
	}
	if err != nil {
		return nil, fmt.Errorf("location: get country %q: %w", code, err)
	}
	return &c, nil
}

func (r *gormRepository) ListStatesByCountry(ctx context.Context, countryCode string) ([]State, error) {
	var out []State
	if err := r.db.WithContext(ctx).
		Where("country_code = ?", countryCode).
		Order("name asc").
		Find(&out).Error; err != nil {
		return nil, fmt.Errorf("location: list states for %q: %w", countryCode, err)
	}
	return out, nil
}

func (r *gormRepository) GetState(ctx context.Context, id string) (*State, error) {
	var s State
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("state_not_found", fmt.Sprintf("state %q does not exist", id))
	}
	if err != nil {
		return nil, fmt.Errorf("location: get state %q: %w", id, err)
	}
	return &s, nil
}

func (r *gormRepository) ListCurrencies(ctx context.Context) ([]Currency, error) {
	var out []Currency
	if err := r.db.WithContext(ctx).Order("code asc").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("location: list currencies: %w", err)
	}
	return out, nil
}

func (r *gormRepository) GetCurrency(ctx context.Context, code string) (*Currency, error) {
	var c Currency
	err := r.db.WithContext(ctx).Where("code = ?", code).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("currency_not_found", fmt.Sprintf("currency %q does not exist", code))
	}
	if err != nil {
		return nil, fmt.Errorf("location: get currency %q: %w", code, err)
	}
	return &c, nil
}

func (r *gormRepository) ListTimezones(ctx context.Context) ([]Timezone, error) {
	var out []Timezone
	if err := r.db.WithContext(ctx).Order("id asc").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("location: list timezones: %w", err)
	}
	return out, nil
}

func (r *gormRepository) GetTimezone(ctx context.Context, id string) (*Timezone, error) {
	var tz Timezone
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&tz).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.NotFound("timezone_not_found", fmt.Sprintf("timezone %q does not exist", id))
	}
	if err != nil {
		return nil, fmt.Errorf("location: get timezone %q: %w", id, err)
	}
	return &tz, nil
}
