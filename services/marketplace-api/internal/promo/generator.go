package promo

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

// charset is the visually-safe alphanumeric character set for promo codes.
// Removed: 0 (confused with O), O (confused with 0), 1 (confused with l/I),
// l (lowercase L, confused with 1/I), I (uppercase i, confused with 1/l).
// All remaining characters are unambiguous at common font sizes (§7.3).
const charset = "23456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz"

// MinCodeLength is the minimum allowed code length per §7.3.
const MinCodeLength = 12

// GenerateCode returns a cryptographically random promo code of the given
// length using only the visually-safe charset defined above.
// Length must be >= MinCodeLength; returns an error otherwise.
func GenerateCode(length int) (string, error) {
	if length < MinCodeLength {
		return "", fmt.Errorf("promo: code length %d is below minimum %d", length, MinCodeLength)
	}
	buf := make([]byte, length)
	n := big.NewInt(int64(len(charset)))
	for i := range buf {
		idx, err := rand.Int(rand.Reader, n)
		if err != nil {
			return "", fmt.Errorf("promo: generate code: %w", err)
		}
		buf[i] = charset[idx.Int64()]
	}
	return string(buf), nil
}
