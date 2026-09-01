package journal_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/journal"
)

func TestNormalizeEmail_TrimsAndLowercases(t *testing.T) {
	require.Equal(t, "ada@example.com", journal.NormalizeEmail("  Ada@Example.COM  "))
}

func TestNormalizeEmail_AlreadyNormalizedIsUnchanged(t *testing.T) {
	require.Equal(t, "ada@example.com", journal.NormalizeEmail("ada@example.com"))
}

func TestNormalizeEmail_EmptyStringStaysEmpty(t *testing.T) {
	require.Equal(t, "", journal.NormalizeEmail("   "))
}

func TestSubscriber_TableName(t *testing.T) {
	require.Equal(t, "journal_subscribers", journal.Subscriber{}.TableName())
}
