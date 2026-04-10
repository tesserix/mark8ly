package csvjob_test

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/csvjob"
)

func TestParseRow(t *testing.T) {
	headers := []string{"title", "handle", "description", "status", "base_price", "sku", "stock", "weight", "category_slugs"}

	tests := []struct {
		name    string
		row     []string
		wantErr string
		check   func(t *testing.T, d csvjob.ProductDraft)
	}{
		{
			name: "valid row",
			row:  []string{"T-Shirt", "t-shirt", "A nice shirt", "active", "29.99", "TSH-001", "100", "200g", "clothing|shirts"},
			check: func(t *testing.T, d csvjob.ProductDraft) {
				require.Equal(t, "T-Shirt", d.Title)
				require.Equal(t, "t-shirt", d.Handle)
				require.Equal(t, "A nice shirt", d.Description)
				require.Equal(t, "active", d.Status)
				require.True(t, d.BasePrice.Equal(decimal.NewFromFloat(29.99)))
				require.Equal(t, "TSH-001", d.SKU)
				require.Equal(t, 100, d.Stock)
				require.Equal(t, "200g", d.Weight)
				require.Equal(t, []string{"clothing", "shirts"}, d.CategorySlugs)
			},
		},
		{
			name:    "missing title",
			row:     []string{"", "handle-1", "", "", "10", "SKU-1", "5", "", ""},
			wantErr: "title is required",
		},
		{
			name:    "missing handle",
			row:     []string{"Title", "", "", "", "10", "SKU-1", "5", "", ""},
			wantErr: "handle is required",
		},
		{
			name:    "invalid price",
			row:     []string{"Title", "handle-2", "", "", "not-a-price", "SKU-2", "5", "", ""},
			wantErr: "not a valid decimal",
		},
		{
			name:    "non-integer stock",
			row:     []string{"Title", "handle-3", "", "", "10", "SKU-3", "3.5", "", ""},
			wantErr: "not a valid integer",
		},
		{
			name: "default status is draft",
			row:  []string{"Title", "handle-4", "", "", "10", "SKU-4", "5", "", ""},
			check: func(t *testing.T, d csvjob.ProductDraft) {
				require.Equal(t, "draft", d.Status)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := csvjob.NewHandleTracker()
			draft, err := csvjob.ParseRow(headers, tt.row, 1, tracker)
			if tt.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			if tt.check != nil {
				tt.check(t, draft)
			}
		})
	}
}

func TestParseRow_DuplicateHandle(t *testing.T) {
	headers := []string{"title", "handle", "base_price", "sku", "stock"}
	tracker := csvjob.NewHandleTracker()

	// First row succeeds.
	_, err := csvjob.ParseRow(headers, []string{"Shirt", "my-handle", "10", "S1", "5"}, 1, tracker)
	require.NoError(t, err)

	// Second row with same handle fails.
	_, err = csvjob.ParseRow(headers, []string{"Pants", "my-handle", "20", "S2", "3"}, 2, tracker)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate handle")
}
