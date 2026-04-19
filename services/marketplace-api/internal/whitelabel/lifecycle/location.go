package lifecycle

import "time"

// TimeLocation aliases time.Location so scheduler.go can reference a
// package-local type without a direct dep on time at the signature
// level — keeps the mustUTC helper typed and short.
type TimeLocation = time.Location

var utcLoc = time.UTC
