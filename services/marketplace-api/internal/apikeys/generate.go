package apikeys

import (
	"crypto/rand"
	"errors"
	"strings"

	"github.com/mr-tron/base58"
)

// Env distinguishes live vs. test keys via the prefix; only Live is wired
// today, but the format reserves mk8_test_ for future test-environment work.
type Env string

const (
	EnvLive Env = "live"
	EnvTest Env = "test"
)

const (
	livePrefix   = "mk8_live_"
	testPrefix   = "mk8_test_"
	prefixLen    = 8
	entropyBytes = 32
)

// Generated holds the freshly-minted plaintext key (returned to the caller
// exactly once) and the prefix that lands in the database.
type Generated struct {
	Plaintext string
	Prefix    string
}

// Generate returns a fresh key. Plaintext is `mk8_{env}_{base58(32 random
// bytes)}`. Prefix is the first 8 chars of the base58 body — what middleware
// uses to do an O(log n) DB lookup before bcrypt verify.
func Generate(env Env) (Generated, error) {
	raw := make([]byte, entropyBytes)
	if _, err := rand.Read(raw); err != nil {
		return Generated{}, err
	}
	body := base58.Encode(raw)
	if len(body) < prefixLen+4 {
		return Generated{}, errors.New("apikeys: unexpectedly short base58 body")
	}
	pfx := livePrefix
	if env == EnvTest {
		pfx = testPrefix
	}
	return Generated{
		Plaintext: pfx + body,
		Prefix:    body[:prefixLen],
	}, nil
}

// ExtractPrefix parses a bearer token and returns the database-side prefix.
// Returns ok=false for malformed tokens (wrong env prefix, too short, etc.).
func ExtractPrefix(key string) (string, bool) {
	body, ok := stripEnvPrefix(key)
	if !ok {
		return "", false
	}
	if len(body) < prefixLen+4 {
		return "", false
	}
	return body[:prefixLen], true
}

func stripEnvPrefix(key string) (string, bool) {
	switch {
	case strings.HasPrefix(key, livePrefix):
		return key[len(livePrefix):], true
	case strings.HasPrefix(key, testPrefix):
		return key[len(testPrefix):], true
	}
	return "", false
}

// Display renders a masked form for UI listings: mk8_live_ABCD****WXYZ
// (env prefix + first 8 chars of body + 4 stars + last 4 chars of body).
// Used for the create-time HTTP response only; never logged.
func Display(key string) string {
	body, ok := stripEnvPrefix(key)
	if !ok || len(body) < prefixLen+4 {
		return ""
	}
	envPrefix := key[:len(key)-len(body)]
	return envPrefix + body[:prefixLen] + "****" + body[len(body)-4:]
}
