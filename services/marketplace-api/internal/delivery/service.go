package delivery

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

const (
	maxOptions = 5
	maxName    = 40
)

// Settings is everything the admin UI needs for the delivery page.
type Settings struct {
	Mode            string           `json:"mode"`
	FreeShippingMin *decimal.Decimal `json:"free_shipping_min"`
	Options         []Option         `json:"options"`
	Couriers        []Courier        `json:"couriers"`
}

// OptionInput is one option as submitted by the admin; ID set means update.
type OptionInput struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Price    *decimal.Decimal `json:"price"` // required
	Estimate *string          `json:"estimate"`
	Enabled  bool             `json:"enabled"`
}

// CourierInput is a courier as submitted by the admin.
type CourierInput struct {
	Name                string  `json:"name"`
	TrackingURLTemplate *string `json:"tracking_url_template"`
}

type Service struct{ db *gorm.DB }

func NewService(db *gorm.DB) *Service { return &Service{db: db} }

// validMoney: 0 to 99,999.99, at most 2 decimals. The exponent check runs first
// so a hostile 1e999999999 is rejected before any arithmetic.
func validMoney(d decimal.Decimal) bool {
	return d.Exponent() >= -2 && d.Exponent() <= 5 && !d.IsNegative() && d.LessThan(decimal.New(1, 5))
}

func invalid(field, msg string) error { return apperrors.ValidationFailed(field, msg) }

func notFound(what string) error { return apperrors.NotFound(what) }

func (s *Service) Get(ctx context.Context, storeID string) (Settings, error) {
	var st struct {
		ShippingMode       string
		OwnFreeShippingMin *decimal.Decimal
	}
	if err := s.db.WithContext(ctx).Table("stores").Select("shipping_mode, own_free_shipping_min").
		Where("id = ?", storeID).Take(&st).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Settings{}, notFound("store")
		}
		return Settings{}, err
	}
	out := Settings{Mode: st.ShippingMode, FreeShippingMin: st.OwnFreeShippingMin, Options: []Option{}}
	if err := s.db.WithContext(ctx).Where("store_id = ?", storeID).Order("position, created_at").Find(&out.Options).Error; err != nil {
		return Settings{}, err
	}
	var err error
	out.Couriers, err = s.ListCouriers(ctx, storeID)
	return out, err
}

func (s *Service) SetMode(ctx context.Context, storeID, mode string) error {
	if mode != "carrier" && mode != "own" {
		return invalid("mode", "must be carrier or own")
	}
	return s.updateStore(ctx, storeID, map[string]any{"shipping_mode": mode})
}

func (s *Service) SetOwnFreeShippingMin(ctx context.Context, storeID string, v *decimal.Decimal) error {
	if err := checkFreeMin(v); err != nil {
		return err
	}
	return s.updateStore(ctx, storeID, map[string]any{"own_free_shipping_min": v})
}

func checkFreeMin(v *decimal.Decimal) error {
	if v != nil && !validMoney(*v) {
		return invalid("free_shipping_min", "must be between 0 and 99999.99 with at most 2 decimals")
	}
	return nil
}

func (s *Service) updateStore(ctx context.Context, storeID string, vals map[string]any) error {
	r := s.db.WithContext(ctx).Table("stores").Where("id = ?", storeID).Updates(vals)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected == 0 {
		return notFound("store")
	}
	return nil
}

func validateOption(i int, o *OptionInput) error {
	f := fmt.Sprintf("options[%d]", i)
	o.Name = strings.TrimSpace(o.Name)
	if o.Name == "" || utf8.RuneCountInString(o.Name) > maxName {
		return invalid(f+".name", "required, at most 40 characters")
	}
	if o.Price == nil || !validMoney(*o.Price) {
		return invalid(f+".price", "required, between 0 and 99999.99 with at most 2 decimals")
	}
	if o.Estimate != nil {
		e := strings.TrimSpace(*o.Estimate)
		if utf8.RuneCountInString(e) > maxName {
			return invalid(f+".estimate", "at most 40 characters")
		}
		if e == "" {
			o.Estimate = nil
		} else {
			o.Estimate = &e
		}
	}
	return nil
}

// ReplaceOptions makes the store's options exactly opts, in array order.
// Options with an id update in place; ids not in opts are deleted. freeMin
// (nil clears it) is written in the same transaction.
func (s *Service) ReplaceOptions(ctx context.Context, tenantID, storeID string, opts []OptionInput, freeMin *decimal.Decimal) ([]Option, error) {
	if err := checkFreeMin(freeMin); err != nil {
		return nil, err
	}
	if len(opts) > maxOptions {
		return nil, invalid("options", "at most 5 delivery options")
	}
	for i := range opts {
		if err := validateOption(i, &opts[i]); err != nil {
			return nil, err
		}
	}
	var out []Option
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if r := tx.Table("stores").Where("id = ?", storeID).Update("own_free_shipping_min", freeMin); r.Error != nil || r.RowsAffected == 0 {
			if r.Error != nil {
				return r.Error
			}
			return notFound("store")
		}
		var existing []Option
		if err := tx.Where("store_id = ?", storeID).Find(&existing).Error; err != nil {
			return err
		}
		have := map[string]bool{}
		for _, e := range existing {
			have[e.ID] = true
		}
		keep := map[string]bool{}
		out = make([]Option, 0, len(opts))
		for i, in := range opts {
			o := Option{TenantID: tenantID, StoreID: storeID, Name: in.Name, Price: *in.Price,
				Estimate: in.Estimate, Enabled: in.Enabled, Position: i}
			if in.ID != "" {
				if !have[in.ID] || keep[in.ID] {
					return invalid(fmt.Sprintf("options[%d].id", i), "unknown option")
				}
				keep[in.ID] = true
				o.ID = in.ID
				if err := tx.Model(&Option{}).Where("id = ? AND store_id = ?", o.ID, storeID).
					Updates(map[string]any{"name": o.Name, "price": o.Price, "estimate": o.Estimate,
						"enabled": o.Enabled, "position": o.Position, "updated_at": gorm.Expr("now()")}).Error; err != nil {
					return err
				}
				if err := tx.Take(&o, "id = ?", o.ID).Error; err != nil {
					return err
				}
			} else {
				o.ID = uuid.NewString()
				if err := tx.Create(&o).Error; err != nil {
					return err
				}
				// gorm swaps a false bool for the column default (true) on insert.
				if !in.Enabled {
					if err := tx.Model(&Option{}).Where("id = ?", o.ID).Update("enabled", false).Error; err != nil {
						return err
					}
					o.Enabled = false
				}
			}
			out = append(out, o)
		}
		var drop []string
		for id := range have {
			if !keep[id] {
				drop = append(drop, id)
			}
		}
		if len(drop) > 0 {
			return tx.Where("store_id = ? AND id IN ?", storeID, drop).Delete(&Option{}).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// EnabledOption returns an enabled option of the store, else NotFound.
func (s *Service) EnabledOption(ctx context.Context, storeID, optionID string) (*Option, error) {
	if _, err := uuid.Parse(optionID); err != nil {
		return nil, notFound("delivery option")
	}
	var o Option
	err := s.db.WithContext(ctx).Where("id = ? AND store_id = ? AND enabled", optionID, storeID).Take(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, notFound("delivery option")
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (s *Service) ListCouriers(ctx context.Context, storeID string) ([]Courier, error) {
	out := []Courier{}
	err := s.db.WithContext(ctx).Where("store_id = ?", storeID).Order("name, created_at").Find(&out).Error
	return out, err
}

// GetCourier returns the store's courier, else NotFound.
func (s *Service) GetCourier(ctx context.Context, storeID, courierID string) (*Courier, error) {
	if _, err := uuid.Parse(courierID); err != nil {
		return nil, notFound("courier")
	}
	var c Courier
	err := s.db.WithContext(ctx).Where("id = ? AND store_id = ?", courierID, storeID).Take(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, notFound("courier")
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func validateCourier(in *CourierInput) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > maxName {
		return invalid("name", "required, at most 40 characters")
	}
	if in.TrackingURLTemplate != nil {
		t := strings.TrimSpace(*in.TrackingURLTemplate)
		if t == "" {
			in.TrackingURLTemplate = nil
			return nil
		}
		if err := ValidateTemplate(t); err != nil {
			return invalid("tracking_url_template", err.Error())
		}
		in.TrackingURLTemplate = &t
	}
	return nil
}

func (s *Service) CreateCourier(ctx context.Context, tenantID, storeID string, in CourierInput) (*Courier, error) {
	if err := validateCourier(&in); err != nil {
		return nil, err
	}
	c := Courier{TenantID: tenantID, StoreID: storeID, Name: in.Name, TrackingURLTemplate: in.TrackingURLTemplate}
	if err := s.db.WithContext(ctx).Create(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Service) UpdateCourier(ctx context.Context, storeID, courierID string, in CourierInput) (*Courier, error) {
	if err := validateCourier(&in); err != nil {
		return nil, err
	}
	c, err := s.GetCourier(ctx, storeID, courierID)
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(c).Updates(map[string]any{
		"name": in.Name, "tracking_url_template": in.TrackingURLTemplate, "updated_at": gorm.Expr("now()")}).Error; err != nil {
		return nil, err
	}
	return s.GetCourier(ctx, storeID, courierID)
}

func (s *Service) DeleteCourier(ctx context.Context, storeID, courierID string) error {
	c, err := s.GetCourier(ctx, storeID, courierID)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Delete(c).Error
}
