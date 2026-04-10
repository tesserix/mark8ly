package loyalty

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTableNames(t *testing.T) {
	assert.Equal(t, "loyalty_programs", LoyaltyProgram{}.TableName())
	assert.Equal(t, "customer_loyalties", CustomerLoyalty{}.TableName())
	assert.Equal(t, "loyalty_transactions", LoyaltyTransaction{}.TableName())
	assert.Equal(t, "referrals", Referral{}.TableName())
}

func TestTierJSON(t *testing.T) {
	tier := Tier{Name: "Gold", MinPoints: 1000, Multiplier: decimal.NewFromFloat(1.5)}
	b, err := json.Marshal(tier)
	require.NoError(t, err)

	var got Tier
	require.NoError(t, json.Unmarshal(b, &got))
	assert.Equal(t, "Gold", got.Name)
	assert.Equal(t, 1000, got.MinPoints)
	assert.True(t, got.Multiplier.Equal(decimal.NewFromFloat(1.5)))
}
