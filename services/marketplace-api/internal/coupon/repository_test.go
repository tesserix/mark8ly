package coupon

import (
	"testing"

	"github.com/google/uuid"
)

// TestNewRepository verifies construction returns a non-nil interface.
func TestNewRepository(t *testing.T) {
	repo := NewRepository()
	if repo == nil {
		t.Fatal("NewRepository() returned nil")
	}
}

// TestListFilter_Defaults verifies sensible defaults for ListFilter.
func TestListFilter_Defaults(t *testing.T) {
	f := ListFilter{
		StoreID:  uuid.New(),
		TenantID: uuid.New(),
	}
	if f.Page != 0 {
		t.Errorf("expected zero-value page, got %d", f.Page)
	}
	// The repository normalises page < 1 to 1 internally.
}
