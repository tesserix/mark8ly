package loyalty

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewExpiryWorker(t *testing.T) {
	w := NewExpiryWorker(nil, NewRepository(), slog.Default())
	assert.NotNil(t, w)
}
