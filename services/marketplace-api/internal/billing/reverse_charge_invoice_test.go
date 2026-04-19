package billing_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/billing"
)

type fakeInvoiceUpdater struct {
	calls []struct {
		invoiceID string
		fields    []billing.CustomField
	}
	err error
}

func (f *fakeInvoiceUpdater) UpdateInvoiceCustomFields(_ context.Context, invoiceID string, fields []billing.CustomField) error {
	f.calls = append(f.calls, struct {
		invoiceID string
		fields    []billing.CustomField
	}{invoiceID, fields})
	return f.err
}

func TestReverseCharge_UKValidated_Annotates(t *testing.T) {
	upd := &fakeInvoiceUpdater{}
	annot := billing.NewReverseChargeAnnotator(upd)

	require.NoError(t, annot.AnnotateIfNeeded(context.Background(), billing.AnnotateInput{
		InvoiceID:          "in_uk_1",
		Country:            "GB",
		TaxIDValidated:     true,
		ReverseChargeTaxID: "GB123456789",
	}))
	require.Len(t, upd.calls, 1)
	require.Equal(t, "in_uk_1", upd.calls[0].invoiceID)
	require.Equal(t, "Tax Treatment", upd.calls[0].fields[0].Name)
	require.Contains(t, upd.calls[0].fields[0].Value, "Reverse charge")
	require.Contains(t, upd.calls[0].fields[0].Value, "GB123456789")
}

func TestReverseCharge_UKUnvalidated_Skips(t *testing.T) {
	upd := &fakeInvoiceUpdater{}
	annot := billing.NewReverseChargeAnnotator(upd)

	require.NoError(t, annot.AnnotateIfNeeded(context.Background(), billing.AnnotateInput{
		InvoiceID: "in_uk_2", Country: "GB", TaxIDValidated: false,
	}))
	require.Empty(t, upd.calls, "unvalidated B2B must not claim reverse charge")
}

func TestReverseCharge_AUValidated_Skips(t *testing.T) {
	upd := &fakeInvoiceUpdater{}
	annot := billing.NewReverseChargeAnnotator(upd)
	require.NoError(t, annot.AnnotateIfNeeded(context.Background(), billing.AnnotateInput{
		InvoiceID: "in_au_1", Country: "AU", TaxIDValidated: true,
	}))
	require.Empty(t, upd.calls, "AU is domestic GST — never reverse charge")
}

func TestReverseCharge_USValidated_Skips(t *testing.T) {
	upd := &fakeInvoiceUpdater{}
	annot := billing.NewReverseChargeAnnotator(upd)
	require.NoError(t, annot.AnnotateIfNeeded(context.Background(), billing.AnnotateInput{
		InvoiceID: "in_us_1", Country: "US", TaxIDValidated: true,
	}))
	require.Empty(t, upd.calls, "US has no federal VAT/sales-tax reverse charge")
}

func TestReverseCharge_AllReverseChargeCountriesAnnotated(t *testing.T) {
	for _, country := range []string{"GB", "IE", "DE", "FR", "IT", "ES", "NL", "IN", "SG", "MY", "TH", "PH", "ID", "VN", "NZ"} {
		upd := &fakeInvoiceUpdater{}
		annot := billing.NewReverseChargeAnnotator(upd)
		err := annot.AnnotateIfNeeded(context.Background(), billing.AnnotateInput{
			InvoiceID: "in_test_" + country, Country: country,
			TaxIDValidated: true, ReverseChargeTaxID: "TAXID-" + country,
		})
		require.NoErrorf(t, err, "country %s", country)
		require.Lenf(t, upd.calls, 1, "country %s expected 1 call", country)
	}
}

func TestReverseCharge_NilStripe_NoOp(t *testing.T) {
	annot := billing.NewReverseChargeAnnotator(nil)
	// Even with a reverse-charge country, nil client should not panic.
	require.NoError(t, annot.AnnotateIfNeeded(context.Background(), billing.AnnotateInput{
		InvoiceID: "in_x", Country: "GB", TaxIDValidated: true,
	}))
}

func TestReverseCharge_EmptyInvoiceID_ReturnsError(t *testing.T) {
	upd := &fakeInvoiceUpdater{}
	annot := billing.NewReverseChargeAnnotator(upd)
	err := annot.AnnotateIfNeeded(context.Background(), billing.AnnotateInput{
		Country: "GB", TaxIDValidated: true, ReverseChargeTaxID: "GB123456789",
	})
	require.Error(t, err)
}
