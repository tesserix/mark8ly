package arbitrage

import (
	"net"

	"github.com/gin-gonic/gin"
)

// cfOpaqueCodes are Cloudflare-specific sentinels that are not real ISO codes.
// We treat them as "unknown" so the evaluator downgrades to a Note rather
// than flagging on bogus data.
var cfOpaqueCodes = map[string]struct{}{
	"XX": {}, // anonymous networks
	"T1": {}, // Tor exit
}

// CFIPCountryFromGin returns (ipCountry, rawIP). It reads the `CF-IPCountry`
// header (populated by Cloudflare Tunnel in front of Istio) and the
// `CF-Connecting-IP` header (the client's public IP as seen by Cloudflare).
//
// IMPORTANT: the rawIP is returned only for HMAC hashing. Callers MUST NOT
// persist, log, or forward it. Task 15 greps for violations.
func CFIPCountryFromGin(c *gin.Context) (ipCountry string, rawIP string) {
	ipCountry = NormalizeCountry(c.GetHeader("CF-IPCountry"))
	if _, opaque := cfOpaqueCodes[ipCountry]; opaque {
		ipCountry = "??"
	}

	rawIP = c.GetHeader("CF-Connecting-IP")
	if rawIP == "" {
		// Fall back to the transport address — useful for tests + local dev.
		host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
		if err == nil {
			rawIP = host
		} else {
			rawIP = c.Request.RemoteAddr
		}
	}
	return ipCountry, rawIP
}
