// Package country wraps the supported_countries table with a
// repository and public HTTP handler for reference data.
package country

import "github.com/lib/pq"

// SupportedCountry is the GORM model for the supported_countries table.
type SupportedCountry struct {
	CountryCode      string         `gorm:"column:country_code;primaryKey;type:char(2)" json:"country_code"`
	Name             string         `gorm:"column:name;type:varchar(100);not null"       json:"name"`
	CurrencyCode     string         `gorm:"column:currency_code;type:char(3);not null"   json:"currency_code"`
	Region           string         `gorm:"column:region;type:varchar(20);not null"       json:"region"`
	PaymentProviders pq.StringArray `gorm:"column:payment_providers;type:text[];not null" json:"payment_providers"`
	ShippingCarriers pq.StringArray `gorm:"column:shipping_carriers;type:text[];not null" json:"shipping_carriers"`
	TaxStrategy      string         `gorm:"column:tax_strategy;type:varchar(20);not null" json:"tax_strategy"`
	TaxRate          *float64       `gorm:"column:tax_rate;type:numeric(5,2)"             json:"tax_rate,omitempty"`
	IsActive         bool           `gorm:"column:is_active;not null;default:true"        json:"-"`
}

// TableName returns the Postgres table name.
func (SupportedCountry) TableName() string { return "supported_countries" }
