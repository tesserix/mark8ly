// Package location provides reference data lookups for countries, states,
// currencies, and timezones. The data is read-only at runtime and seeded
// from JSON files via cmd/seed.
//
// Design notes (clean-up vs old location-service):
//   - No soft-deletes. This is reference data; rows are inserted once and
//     never deleted in normal operation. If a country becomes obsolete the
//     migration drops the row.
//   - No JSON-as-string columns. Languages and country lists are real arrays
//     using Postgres `text[]`.
//   - No `Active` column. Inactive rows would just be deleted from the seed.
//     Adds complexity for no value at the platform-api scale.
//   - No GORM hooks. Pure data models.
//   - PK is the natural ISO code, not a synthetic UUID. Stable, readable,
//     human-debuggable.
package location

import "time"

// Country is an ISO 3166-1 country.
type Country struct {
	// Code is the ISO 3166-1 alpha-2 country code (e.g. "US", "IN").
	Code string `gorm:"primaryKey;column:code;type:char(2)"     json:"code"`
	Name string `gorm:"column:name;type:varchar(100);not null"  json:"name"`
	// NativeName is the country's name in its primary local language.
	NativeName string `gorm:"column:native_name;type:varchar(100)"      json:"native_name"`
	// CallingCode is the international dialing prefix including the leading "+".
	CallingCode string `gorm:"column:calling_code;type:varchar(10)"      json:"calling_code"`
	// CurrencyCode is the ISO 4217 code of the country's primary currency.
	CurrencyCode string `gorm:"column:currency_code;type:char(3)"         json:"currency_code"`
	FlagEmoji    string `gorm:"column:flag_emoji;type:varchar(10)"        json:"flag_emoji"`
	Region       string `gorm:"column:region;type:varchar(50)"            json:"region"`

	CreatedAt time.Time `gorm:"column:created_at;not null;default:now()" json:"-"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;default:now()" json:"-"`
}

// TableName overrides GORM's default pluralization to match the migration.
func (Country) TableName() string { return "countries" }

// State is a country's first-level administrative subdivision (state, province, region).
type State struct {
	// ID is the country code + state code joined by "-" (e.g. "US-CA", "IN-MH").
	ID string `gorm:"primaryKey;column:id;type:varchar(10)"  json:"id"`
	// CountryCode is the parent country's ISO code.
	CountryCode string `gorm:"column:country_code;type:char(2);not null;index" json:"country_code"`
	// Code is the state's local code (e.g. "CA", "MH").
	Code string `gorm:"column:code;type:varchar(10);not null"  json:"code"`
	Name string `gorm:"column:name;type:varchar(100);not null" json:"name"`
	// Type is one of: state, province, territory, region, district.
	Type string `gorm:"column:type;type:varchar(20);not null;default:'state'" json:"type"`

	CreatedAt time.Time `gorm:"column:created_at;not null;default:now()" json:"-"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;default:now()" json:"-"`
}

// TableName overrides GORM's default pluralization.
func (State) TableName() string { return "states" }

// Currency is an ISO 4217 currency.
type Currency struct {
	// Code is the ISO 4217 alpha-3 currency code (e.g. "USD", "INR").
	Code string `gorm:"primaryKey;column:code;type:char(3)"      json:"code"`
	Name string `gorm:"column:name;type:varchar(100);not null"   json:"name"`
	// Symbol is the currency's display symbol (e.g. "$", "₹"). Not unique.
	Symbol        string `gorm:"column:symbol;type:varchar(10);not null"  json:"symbol"`
	DecimalPlaces int    `gorm:"column:decimal_places;not null;default:2" json:"decimal_places"`

	CreatedAt time.Time `gorm:"column:created_at;not null;default:now()" json:"-"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;default:now()" json:"-"`
}

// TableName overrides GORM's default pluralization.
func (Currency) TableName() string { return "currencies" }

// Timezone is an IANA timezone.
type Timezone struct {
	// ID is the IANA timezone identifier (e.g. "America/New_York", "Asia/Kolkata").
	ID   string `gorm:"primaryKey;column:id;type:varchar(64)"   json:"id"`
	Name string `gorm:"column:name;type:varchar(100);not null"  json:"name"`
	// UTCOffset is the standard-time offset from UTC (e.g. "-05:00", "+05:30").
	UTCOffset string `gorm:"column:utc_offset;type:varchar(6);not null" json:"utc_offset"`

	CreatedAt time.Time `gorm:"column:created_at;not null;default:now()" json:"-"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;default:now()" json:"-"`
}

// TableName overrides GORM's default pluralization.
func (Timezone) TableName() string { return "timezones" }
