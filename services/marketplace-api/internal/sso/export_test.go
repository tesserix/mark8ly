// export_test.go exposes internal constructors for white-box testing.
// This file is compiled only when running `go test` — the build tag is
// implicit for files named *_test.go in the same package.
package sso

// GIPAdminClientIface is the exported alias of the unexported gipAdminClient
// interface so that external _test packages can implement it.
type GIPAdminClientIface = gipAdminClient

// NewGIPClientFromFake is a test-only constructor that accepts any value
// satisfying the unexported gipAdminClient interface.  It is used by the
// sso_test package to inject fakeGIPAdminClient without the real Firebase SDK.
func NewGIPClientFromFake(client GIPAdminClientIface, poolID string) *GIPClient {
	return NewGIPClient(client, poolID)
}
