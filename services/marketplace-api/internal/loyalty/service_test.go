package loyalty

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestValidateTiers_Valid(t *testing.T) {
	tiers := []Tier{
		{Name: "Bronze", MinPoints: 0, Multiplier: decimal.NewFromInt(1)},
		{Name: "Silver", MinPoints: 500, Multiplier: decimal.NewFromFloat(1.5)},
		{Name: "Gold", MinPoints: 1000, Multiplier: decimal.NewFromInt(2)},
	}
	assert.NoError(t, validateTiers(tiers))
}

func TestValidateTiers_TooMany(t *testing.T) {
	tiers := make([]Tier, 5)
	for i := range tiers {
		tiers[i] = Tier{Name: "T" + string(rune('A'+i)), MinPoints: i * 100, Multiplier: decimal.NewFromInt(1)}
	}
	err := validateTiers(tiers)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "maximum 4 tiers")
}

func TestValidateTiers_DuplicateNames(t *testing.T) {
	tiers := []Tier{
		{Name: "Gold", MinPoints: 0, Multiplier: decimal.NewFromInt(1)},
		{Name: "Gold", MinPoints: 100, Multiplier: decimal.NewFromInt(2)},
	}
	err := validateTiers(tiers)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate name")
}

func TestValidateTiers_NonAscendingMinPoints(t *testing.T) {
	tiers := []Tier{
		{Name: "Silver", MinPoints: 500, Multiplier: decimal.NewFromFloat(1.5)},
		{Name: "Bronze", MinPoints: 0, Multiplier: decimal.NewFromInt(1)},
	}
	err := validateTiers(tiers)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ascending")
}

func TestValidateTiers_ZeroMultiplier(t *testing.T) {
	tiers := []Tier{
		{Name: "Bronze", MinPoints: 0, Multiplier: decimal.Zero},
	}
	err := validateTiers(tiers)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "multiplier")
}

func TestValidateTiers_Empty(t *testing.T) {
	assert.NoError(t, validateTiers(nil))
	assert.NoError(t, validateTiers([]Tier{}))
}
