package product

import (
	"sort"
	"strings"

	"github.com/mark8ly/marketplace-api/pkg/apperrors"
)

// OptionSpec is the in-memory shape of an option axis for matrix validation.
// Separate from gorm's Option model so the service layer can validate
// input before any rows exist in the DB.
type OptionSpec struct {
	Name   string
	Values []OptionValueSpec
}

// OptionValueSpec is one value on an option axis. ID is optional: populated
// on updates where an existing value is being preserved, empty on Create.
type OptionValueSpec struct {
	ID    string
	Value string
}

// VariantSpec is the in-memory shape of a single variant for matrix
// validation. OptionValueRefs names the tuple of (option name -> value)
// that identifies this variant within the matrix.
type VariantSpec struct {
	ID           string // optional; empty = new
	SKU          string
	OptionValues []OptionValueRef // one entry per option axis
}

// OptionValueRef identifies an option axis + value pair on a variant.
type OptionValueRef struct {
	OptionName string
	Value      string
}

// MaxVariantsPerProduct is the hard upper bound on variants per product
// (M7c gap #7). Enforced in ValidateMatrix so both Create and Update
// paths reject over-cap matrices before any DB work.
const MaxVariantsPerProduct = 500

// ValidateMatrix asserts the option/variant shape is well-formed per
// spec §14.17 caps:
//   - <=3 option axes
//   - <=MaxVariantsPerProduct variants
//   - variant count == Pi(len(option.values))
//   - every variant names exactly each option once with a value that
//     exists on that option
func ValidateMatrix(options []OptionSpec, variants []VariantSpec) error {
	if len(options) > 3 {
		return apperrors.TooManyOptions(len(options))
	}
	if len(variants) > MaxVariantsPerProduct {
		return apperrors.TooManyVariants(len(variants))
	}

	if len(options) == 0 {
		if len(variants) != 1 {
			return apperrors.VariantMatrixMismatch(1, len(variants))
		}
		return nil
	}

	valuesByOption := make(map[string]map[string]struct{}, len(options))
	expected := 1
	for _, opt := range options {
		if len(opt.Values) == 0 {
			return apperrors.ValidationFailed("options", "option axis has no values")
		}
		set := make(map[string]struct{}, len(opt.Values))
		for _, v := range opt.Values {
			set[v.Value] = struct{}{}
		}
		valuesByOption[opt.Name] = set
		expected *= len(opt.Values)
	}
	if expected != len(variants) {
		return apperrors.VariantMatrixMismatch(expected, len(variants))
	}

	for i, v := range variants {
		if len(v.OptionValues) != len(options) {
			return apperrors.ValidationFailed("variants",
				"variant at index "+itoa(i)+" does not name every option axis")
		}
		seen := make(map[string]struct{}, len(options))
		for _, ref := range v.OptionValues {
			if _, dup := seen[ref.OptionName]; dup {
				return apperrors.ValidationFailed("variants",
					"variant at index "+itoa(i)+" names option axis twice")
			}
			seen[ref.OptionName] = struct{}{}
			validValues, ok := valuesByOption[ref.OptionName]
			if !ok {
				return apperrors.ValidationFailed("variants",
					"variant at index "+itoa(i)+" references unknown option "+ref.OptionName)
			}
			if _, ok := validValues[ref.Value]; !ok {
				return apperrors.ValidationFailed("variants",
					"variant at index "+itoa(i)+" references unknown value on "+ref.OptionName)
			}
		}
	}
	return nil
}

// MatrixDiff describes the delta between an existing set of variants and
// an incoming set. Match is by the sorted tuple of (option name, value).
type MatrixDiff struct {
	Adds    []VariantSpec
	Updates []VariantSpec
	Removes []string
}

// DiffMatrix computes the set difference by tuple match. Existing entries
// must have an ID; incoming entries with matching tuples carry over that ID.
func DiffMatrix(existing, incoming []VariantSpec) MatrixDiff {
	existingByKey := make(map[string]VariantSpec, len(existing))
	for _, v := range existing {
		existingByKey[tupleKey(v)] = v
	}
	incomingByKey := make(map[string]VariantSpec, len(incoming))
	for _, v := range incoming {
		incomingByKey[tupleKey(v)] = v
	}

	var diff MatrixDiff
	for key, inVar := range incomingByKey {
		if exVar, ok := existingByKey[key]; ok {
			updated := inVar
			updated.ID = exVar.ID
			diff.Updates = append(diff.Updates, updated)
		} else {
			diff.Adds = append(diff.Adds, inVar)
		}
	}
	for key, exVar := range existingByKey {
		if _, ok := incomingByKey[key]; !ok {
			diff.Removes = append(diff.Removes, exVar.ID)
		}
	}

	sort.Slice(diff.Adds, func(i, j int) bool { return tupleKey(diff.Adds[i]) < tupleKey(diff.Adds[j]) })
	sort.Slice(diff.Updates, func(i, j int) bool { return tupleKey(diff.Updates[i]) < tupleKey(diff.Updates[j]) })
	sort.Strings(diff.Removes)
	return diff
}

func tupleKey(v VariantSpec) string {
	parts := make([]string, len(v.OptionValues))
	for i, r := range v.OptionValues {
		parts[i] = r.OptionName + "=" + r.Value
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
