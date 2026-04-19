package ipprivacy_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/ipprivacy"
)

func TestHash_Deterministic(t *testing.T) {
	h := ipprivacy.New([]byte("test-key-1"))
	a := h.Hash("203.0.113.5")
	b := h.Hash("203.0.113.5")
	require.Equal(t, a, b)
	require.Len(t, a, 64)
}

func TestHash_DifferentKeysProduceDifferentOutput(t *testing.T) {
	a := ipprivacy.New([]byte("key-1")).Hash("203.0.113.5")
	b := ipprivacy.New([]byte("key-2")).Hash("203.0.113.5")
	require.NotEqual(t, a, b)
}

func TestHash_StripsPort(t *testing.T) {
	h := ipprivacy.New([]byte("k"))
	require.Equal(t, h.Hash("203.0.113.5"), h.Hash("203.0.113.5:54321"))
}

func TestHash_EmptyKey_ReturnsEmpty(t *testing.T) {
	require.Equal(t, "", ipprivacy.New(nil).Hash("203.0.113.5"))
	require.Equal(t, "", ipprivacy.New([]byte{}).Hash("203.0.113.5"))
}

func TestHash_NilHasher_ReturnsEmpty(t *testing.T) {
	var h *ipprivacy.Hasher
	require.Equal(t, "", h.Hash("203.0.113.5"))
}

func TestHash_InvalidIP_ReturnsEmpty(t *testing.T) {
	h := ipprivacy.New([]byte("k"))
	require.Equal(t, "", h.Hash(""))
	require.Equal(t, "", h.Hash("not-an-ip"))
}
