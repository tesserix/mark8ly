package product_test

import (
	"errors"
	"testing"

	"github.com/mark8ly/marketplace-api/internal/product"
	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

func asAppErr(t *testing.T, err error) *apperrors.Error {
	t.Helper()
	var ae *apperrors.Error
	if !errors.As(err, &ae) {
		t.Fatalf("expected *apperrors.Error, got %T: %v", err, err)
	}
	return ae
}

func variant(pairs ...string) product.VariantSpec {
	refs := make([]product.OptionValueRef, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		refs = append(refs, product.OptionValueRef{OptionName: pairs[i], Value: pairs[i+1]})
	}
	return product.VariantSpec{OptionValues: refs}
}

func opt(name string, values ...string) product.OptionSpec {
	vs := make([]product.OptionValueSpec, len(values))
	for i, v := range values {
		vs[i] = product.OptionValueSpec{Value: v}
	}
	return product.OptionSpec{Name: name, Values: vs}
}

func TestValidateMatrix_NoOptions_OneVariant_OK(t *testing.T) {
	if err := product.ValidateMatrix(nil, []product.VariantSpec{{SKU: "only"}}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateMatrix_NoOptions_ZeroVariants_Mismatch(t *testing.T) {
	err := product.ValidateMatrix(nil, nil)
	if !errors.Is(err, apperrors.ErrVariantMatrixMismatch) {
		t.Fatalf("want mismatch, got %v", err)
	}
	ae := asAppErr(t, err)
	if ae.Details["expected"] != 1 || ae.Details["got"] != 0 {
		t.Fatalf("bad details: %v", ae.Details)
	}
}

func TestValidateMatrix_NoOptions_TwoVariants_Mismatch(t *testing.T) {
	err := product.ValidateMatrix(nil, []product.VariantSpec{{}, {}})
	if !errors.Is(err, apperrors.ErrVariantMatrixMismatch) {
		t.Fatalf("want mismatch, got %v", err)
	}
}

func TestValidateMatrix_OneOption_ThreeValues_ThreeVariants_OK(t *testing.T) {
	opts := []product.OptionSpec{opt("Size", "S", "M", "L")}
	vs := []product.VariantSpec{
		variant("Size", "S"),
		variant("Size", "M"),
		variant("Size", "L"),
	}
	if err := product.ValidateMatrix(opts, vs); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestValidateMatrix_TwoOptions_2x3_SixVariants_OK(t *testing.T) {
	opts := []product.OptionSpec{opt("Size", "S", "M"), opt("Color", "R", "G", "B")}
	var vs []product.VariantSpec
	for _, s := range []string{"S", "M"} {
		for _, c := range []string{"R", "G", "B"} {
			vs = append(vs, variant("Size", s, "Color", c))
		}
	}
	if err := product.ValidateMatrix(opts, vs); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestValidateMatrix_TwoOptions_2x3_FiveVariants_Mismatch(t *testing.T) {
	opts := []product.OptionSpec{opt("Size", "S", "M"), opt("Color", "R", "G", "B")}
	vs := []product.VariantSpec{
		variant("Size", "S", "Color", "R"),
		variant("Size", "S", "Color", "G"),
		variant("Size", "S", "Color", "B"),
		variant("Size", "M", "Color", "R"),
		variant("Size", "M", "Color", "G"),
	}
	err := product.ValidateMatrix(opts, vs)
	if !errors.Is(err, apperrors.ErrVariantMatrixMismatch) {
		t.Fatalf("want mismatch, got %v", err)
	}
	ae := asAppErr(t, err)
	if ae.Details["expected"] != 6 || ae.Details["got"] != 5 {
		t.Fatalf("bad details: %v", ae.Details)
	}
}

func TestValidateMatrix_FourOptions_Rejected(t *testing.T) {
	opts := []product.OptionSpec{
		opt("A", "1"), opt("B", "1"), opt("C", "1"), opt("D", "1"),
	}
	err := product.ValidateMatrix(opts, nil)
	if !errors.Is(err, apperrors.ErrTooManyOptions) {
		t.Fatalf("want too many options, got %v", err)
	}
	ae := asAppErr(t, err)
	if ae.Details["got"] != 4 {
		t.Fatalf("bad details: %v", ae.Details)
	}
}

func TestValidateMatrix_501Variants_Rejected(t *testing.T) {
	values := make([]string, 501)
	for i := range values {
		values[i] = "v" + itoaTest(i)
	}
	opts := []product.OptionSpec{opt("Big", values...)}
	vs := make([]product.VariantSpec, 501)
	for i := range vs {
		vs[i] = variant("Big", "v"+itoaTest(i))
	}
	err := product.ValidateMatrix(opts, vs)
	if !errors.Is(err, apperrors.ErrTooManyVariants) {
		t.Fatalf("want too many variants, got %v", err)
	}
	ae := asAppErr(t, err)
	if ae.Details["got"] != 501 {
		t.Fatalf("bad details: %v", ae.Details)
	}
}

func TestValidateMatrix_500Variants_Accepted(t *testing.T) {
	values := make([]string, 500)
	for i := range values {
		values[i] = "v" + itoaTest(i)
	}
	opts := []product.OptionSpec{opt("Big", values...)}
	vs := make([]product.VariantSpec, 500)
	for i := range vs {
		vs[i] = variant("Big", "v"+itoaTest(i))
	}
	if err := product.ValidateMatrix(opts, vs); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestValidateMatrix_VariantRefsUnknownOption_Rejected(t *testing.T) {
	opts := []product.OptionSpec{opt("Size", "S", "M")}
	vs := []product.VariantSpec{variant("Color", "Red"), variant("Size", "M")}
	err := product.ValidateMatrix(opts, vs)
	if !errors.Is(err, apperrors.ErrValidationFailed) {
		t.Fatalf("want validation failed, got %v", err)
	}
}

func TestValidateMatrix_VariantRefsUnknownValue_Rejected(t *testing.T) {
	opts := []product.OptionSpec{opt("Size", "S", "M")}
	vs := []product.VariantSpec{variant("Size", "S"), variant("Size", "XL")}
	err := product.ValidateMatrix(opts, vs)
	if !errors.Is(err, apperrors.ErrValidationFailed) {
		t.Fatalf("want validation failed, got %v", err)
	}
}

func TestDiffMatrix_Empty_Empty(t *testing.T) {
	d := product.DiffMatrix(nil, nil)
	if len(d.Adds) != 0 || len(d.Updates) != 0 || len(d.Removes) != 0 {
		t.Fatalf("want all empty, got %+v", d)
	}
}

func TestDiffMatrix_Add_Update_Remove(t *testing.T) {
	A := product.VariantSpec{ID: "1", SKU: "A-sku", OptionValues: []product.OptionValueRef{{OptionName: "Size", Value: "S"}}}
	B := product.VariantSpec{ID: "2", SKU: "B-sku", OptionValues: []product.OptionValueRef{{OptionName: "Size", Value: "M"}}}
	Bupd := product.VariantSpec{SKU: "B-new", OptionValues: []product.OptionValueRef{{OptionName: "Size", Value: "M"}}}
	C := product.VariantSpec{SKU: "C-sku", OptionValues: []product.OptionValueRef{{OptionName: "Size", Value: "L"}}}

	d := product.DiffMatrix([]product.VariantSpec{A, B}, []product.VariantSpec{Bupd, C})

	if len(d.Adds) != 1 || d.Adds[0].SKU != "C-sku" {
		t.Fatalf("adds: %+v", d.Adds)
	}
	if len(d.Updates) != 1 || d.Updates[0].SKU != "B-new" || d.Updates[0].ID != "2" {
		t.Fatalf("updates: %+v", d.Updates)
	}
	if len(d.Removes) != 1 || d.Removes[0] != "1" {
		t.Fatalf("removes: %+v", d.Removes)
	}
}

func TestDiffMatrix_PreservesID_OnMatch(t *testing.T) {
	ex := product.VariantSpec{ID: "keep-me", OptionValues: []product.OptionValueRef{{OptionName: "Size", Value: "S"}}}
	in := product.VariantSpec{SKU: "new-sku", OptionValues: []product.OptionValueRef{{OptionName: "Size", Value: "S"}}}
	d := product.DiffMatrix([]product.VariantSpec{ex}, []product.VariantSpec{in})
	if len(d.Updates) != 1 || d.Updates[0].ID != "keep-me" {
		t.Fatalf("expected ID preserved, got %+v", d.Updates)
	}
}

// itoaTest is a tiny int->string helper for test fixtures; keeps tests
// self-contained without importing strconv just for the matrix tests.
func itoaTest(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
