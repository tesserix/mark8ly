package apikeys_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/apikeys"
)

func TestCache_HitWithinTTL(t *testing.T) {
	c := apikeys.NewCache(60 * time.Second)
	id := uuid.New()
	c.Put("mk8_live_abc", apikeys.CacheEntry{KeyID: id, Scopes: []string{"products:read"}})
	got, ok := c.Get("mk8_live_abc")
	require.True(t, ok)
	require.Equal(t, id, got.KeyID)
}

func TestCache_MissAfterInvalidate(t *testing.T) {
	c := apikeys.NewCache(60 * time.Second)
	c.Put("mk8_live_abc", apikeys.CacheEntry{KeyID: uuid.New()})
	c.Invalidate("mk8_live_abc")
	_, ok := c.Get("mk8_live_abc")
	require.False(t, ok)
}

func TestCache_MissAfterTTL(t *testing.T) {
	c := apikeys.NewCache(10 * time.Millisecond)
	c.Put("mk8_live_abc", apikeys.CacheEntry{KeyID: uuid.New()})
	time.Sleep(20 * time.Millisecond)
	_, ok := c.Get("mk8_live_abc")
	require.False(t, ok)
}

func TestCache_MissForUnknownKey(t *testing.T) {
	c := apikeys.NewCache(60 * time.Second)
	_, ok := c.Get("never-stored")
	require.False(t, ok)
}

func TestCache_InvalidateByKeyID_DropsAllReferences(t *testing.T) {
	c := apikeys.NewCache(60 * time.Second)
	id := uuid.New()
	other := uuid.New()
	c.Put("token-a", apikeys.CacheEntry{KeyID: id})
	c.Put("token-b", apikeys.CacheEntry{KeyID: id})
	c.Put("token-c", apikeys.CacheEntry{KeyID: other})

	c.InvalidateByKeyID(id)
	require.Equal(t, 1, c.Size())
	_, ok := c.Get("token-c")
	require.True(t, ok)
}

func TestCache_PutOverwrites(t *testing.T) {
	c := apikeys.NewCache(60 * time.Second)
	first := uuid.New()
	second := uuid.New()
	c.Put("k", apikeys.CacheEntry{KeyID: first})
	c.Put("k", apikeys.CacheEntry{KeyID: second})

	got, ok := c.Get("k")
	require.True(t, ok)
	require.Equal(t, second, got.KeyID)
}
