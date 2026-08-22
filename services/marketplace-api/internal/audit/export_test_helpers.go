package audit

import "github.com/gin-gonic/gin"

// BuildEntryForTest exposes buildEntry to the package's external tests.
// buildEntry stays unexported: it is an internal detail of Emit.
func BuildEntryForTest(c *gin.Context, ev Event) *Entry { return buildEntry(c, ev) }
