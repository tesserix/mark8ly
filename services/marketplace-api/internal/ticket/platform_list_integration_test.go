//go:build integration

package ticket_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/mark8ly/marketplace-api/internal/ticket"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// seedTicket inserts a support_tickets row via GORM (never a raw
// `INSERT INTO tickets` — that would silently seed the unrelated
// platform-engineering table). NOT NULL columns beyond the filter fields
// (tenant_id, store_id, status, priority) are ticket_number, subject,
// description, submitted_by_name, submitted_by_email — see
// internal/ticket/models.go and migrations/000089_customer_support_tickets.up.sql.
func seedTicket(t *testing.T, db *gorm.DB, tenantID, storeID uuid.UUID, status, priority, subject string) {
	t.Helper()

	tk := ticket.Ticket{
		TenantID:         tenantID,
		StoreID:          storeID,
		TicketNumber:     "TKT-" + uuid.New().String()[:8],
		Subject:          subject,
		Description:      "seeded description for " + subject,
		Status:           ticket.TicketStatus(status),
		Priority:         ticket.TicketPriority(priority),
		SubmittedByName:  "Test Customer",
		SubmittedByEmail: "customer-" + uuid.New().String()[:8] + "@example.com",
	}
	require.NoError(t, db.Create(&tk).Error)
}

// TestListPlatform_SpansStoresAndTenants is the whole point of the method.
// Two tickets under two different stores in two different tenants must both
// come back from one unfiltered call. A fixture with one store would pass
// against the store-scoped List too, and prove nothing.
func TestListPlatform_SpansStoresAndTenants(t *testing.T) {
	db := testdb.NewDB(t, "support_tickets")
	repo := ticket.NewRepository()

	tenantA, storeA := uuid.New(), uuid.New()
	tenantB, storeB := uuid.New(), uuid.New()
	seedTicket(t, db, tenantA, storeA, "open", "high", "Alpha subject")
	seedTicket(t, db, tenantB, storeB, "open", "low", "Beta subject")

	got, err := repo.ListPlatform(context.Background(), db, ticket.PlatformListFilter{Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(2), got.Total)

	subjects := map[string]bool{}
	for _, tk := range got.Tickets {
		subjects[tk.Subject] = true
	}
	require.True(t, subjects["Alpha subject"], "ticket from tenant A / store A must appear")
	require.True(t, subjects["Beta subject"], "ticket from tenant B / store B must appear")
}

// store_id NARROWS; it is not a required scope. Both directions asserted,
// because a filter that always applies and a filter that never applies both
// pass a one-sided test.
func TestListPlatform_StoreIDNarrowsRatherThanScopes(t *testing.T) {
	db := testdb.NewDB(t, "support_tickets")
	repo := ticket.NewRepository()

	tenantA, storeA := uuid.New(), uuid.New()
	tenantB, storeB := uuid.New(), uuid.New()
	seedTicket(t, db, tenantA, storeA, "open", "high", "Alpha subject")
	seedTicket(t, db, tenantB, storeB, "open", "low", "Beta subject")

	all, err := repo.ListPlatform(context.Background(), db, ticket.PlatformListFilter{Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(2), all.Total, "unset store_id must return every store")

	narrowed, err := repo.ListPlatform(context.Background(), db,
		ticket.PlatformListFilter{StoreID: &storeA, Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), narrowed.Total)
	require.Equal(t, "Alpha subject", narrowed.Tickets[0].Subject)
}

// The existing store-scoped List must STAY fail-safe. If someone later makes a
// zero StoreID mean "all stores", this fails — which is the point.
func TestList_ZeroStoreIDStillMatchesNothing(t *testing.T) {
	db := testdb.NewDB(t, "support_tickets")
	repo := ticket.NewRepository()

	seedTicket(t, db, uuid.New(), uuid.New(), "open", "high", "Alpha subject")

	got, err := repo.List(context.Background(), db, ticket.ListFilter{PerPage: 50})
	require.NoError(t, err)
	require.Equal(t, int64(0), got.Total,
		"a zero StoreID must match NOTHING; 'all stores' would be fail-open on the merchant path")
}

func TestListPlatform_FiltersByStatusAndPriority(t *testing.T) {
	db := testdb.NewDB(t, "support_tickets")
	repo := ticket.NewRepository()
	tenant, store := uuid.New(), uuid.New()

	seedTicket(t, db, tenant, store, "open", "high", "Open high")
	seedTicket(t, db, tenant, store, "resolved", "high", "Resolved high")
	seedTicket(t, db, tenant, store, "open", "low", "Open low")

	byStatus, err := repo.ListPlatform(context.Background(), db,
		ticket.PlatformListFilter{Status: "open", Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(2), byStatus.Total)

	byPriority, err := repo.ListPlatform(context.Background(), db,
		ticket.PlatformListFilter{Status: "open", Priority: "low", Limit: 50})
	require.NoError(t, err)
	require.Equal(t, int64(1), byPriority.Total)
	require.Equal(t, "Open low", byPriority.Tickets[0].Subject)
}
