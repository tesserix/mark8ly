//go:build integration

package orderdoc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/branding"
	"github.com/mark8ly/marketplace-api/internal/order"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// captureMailer records the DocumentInput the service built.
type captureMailer struct{ last DocumentInput }

func (m *captureMailer) SendInvoice(_ context.Context, in DocumentInput) error {
	m.last = in
	return nil
}
func (m *captureMailer) SendReceipt(context.Context, DocumentInput) error      { return nil }
func (m *captureMailer) SendCancellation(context.Context, DocumentInput) error { return nil }
func (m *captureMailer) SendRefund(context.Context, DocumentInput) error       { return nil }
func (m *captureMailer) SendShipmentDispatched(context.Context, DocumentInput) error {
	return nil
}

// TestIntegration_BuildInput_CarriesStoreSenderIdentity pins the hop the
// mailer-level test in internal/email cannot see (#718): that test hands
// the mailer a DocumentInput it built itself, so it proves the mailer
// APPLIES an identity, not that the service SUPPLIES one. Blanking
// StoreContactEmail here left every other test in this service green.
func TestIntegration_BuildInput_CarriesStoreSenderIdentity(t *testing.T) {
	tx := testdb.NewTx(t)
	ctx := context.Background()

	tenantID, storeID := uuid.New(), uuid.New()
	testdb.SeedStore(t, tx, tenantID, storeID)
	require.NoError(t, tx.Exec(`
		INSERT INTO store_branding (tenant_id, store_id, support_email)
		VALUES (?, ?, ?)`, tenantID, storeID, "hello@nadiasceramics.com").Error)

	orderID := uuid.New()
	require.NoError(t, tx.Exec(`
		INSERT INTO orders (id, tenant_id, store_id, order_number, idempotency_key,
		                    customer_email, subtotal, grand_total, currency_code)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		orderID, tenantID, storeID, "ORD-718-1", uuid.NewString(),
		"buyer@example.com", "42.00", "42.00", "EUR").Error)

	mailer := &captureMailer{}
	svc := NewService(tx, mailer, order.NewRepository(),
		branding.NewService(branding.ServiceConfig{DB: tx, Repo: branding.NewRepository()}),
		"https://{slug}.mark8ly.com")

	require.NoError(t, svc.SendInvoice(ctx, orderID))

	require.Equal(t, "Test Store", mailer.last.Theme.StoreName,
		"the envelope's From display name comes from this")
	require.NotEmpty(t, mailer.last.StoreSlug,
		"the envelope's From local part is derived from this")
	require.Equal(t, "hello@nadiasceramics.com", mailer.last.StoreContactEmail,
		"the envelope's Reply-To comes from this")
	require.Equal(t, tenantID.String(), mailer.last.TenantID)
}
