//go:build integration

package order_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/internal/outbox"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

func TestIntegration_OrderCreate_InStoreFieldsPersist(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewDB(t, withinTxTruncateTables...)
	storeID, tenantID := uuid.New(), uuid.New()
	seedStore(t, db, storeID)
	svc := order.NewService(db, order.NewRepository(), outbox.NewRepository(db))

	tendered := decimal.RequireFromString("50.00")
	in := baseCreateInput(storeID, tenantID, "instore-"+uuid.NewString())
	in.CustomerEmail = ""
	in.InStore = &order.InStoreDetails{PaymentMethod: "cash", SoldByUserID: "u-1", CashTendered: &tendered}

	res, err := svc.Create(ctx, in)
	require.NoError(t, err)

	var got order.Order
	require.NoError(t, db.First(&got, "id = ?", res.Order.ID).Error)
	require.Equal(t, order.ChannelInStore, got.Channel)
	require.NotNil(t, got.PaymentMethod)
	require.Equal(t, "cash", *got.PaymentMethod)
	require.NotNil(t, got.SoldByUserID)
	require.Equal(t, "u-1", *got.SoldByUserID)
	require.NotNil(t, got.CashTendered)
	require.True(t, got.CashTendered.Equal(tendered))
	require.Equal(t, "", got.CustomerEmail)
}

func TestIntegration_OrderCreate_DefaultsOnline(t *testing.T) {
	ctx := context.Background()
	db := testdb.NewDB(t, withinTxTruncateTables...)
	storeID, tenantID := uuid.New(), uuid.New()
	seedStore(t, db, storeID)
	svc := order.NewService(db, order.NewRepository(), outbox.NewRepository(db))

	res, err := svc.Create(ctx, baseCreateInput(storeID, tenantID, "online-"+uuid.NewString()))
	require.NoError(t, err)

	var got order.Order
	require.NoError(t, db.First(&got, "id = ?", res.Order.ID).Error)
	require.Equal(t, order.ChannelOnline, got.Channel)
	require.Nil(t, got.PaymentMethod)
	require.Nil(t, got.SoldByUserID)
}
