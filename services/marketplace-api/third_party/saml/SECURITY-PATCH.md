# Local SAML security patch

Runtime source copied unchanged from `github.com/crewjam/saml@v0.5.1`, except
for the namespace-qualified `Subject.NameID` XML tag in `schema.go`. The
upstream BSD license is preserved. Unused IdP/example packages and upstream
test/fuzz fixtures are not included.

GO-2026-6909 has no upstream patched release as of 2026-11-06. The foreign
namespace principal-spoofing behavior reproduces against v0.5.1. The local
patch ensures only SAML assertion namespace NameID elements populate the
principal. Missing issuers in this exact version are value types, not pointers;
the existing issuer comparison rejects them against our required nonempty IdP
entity ID, without the nil dereference described by the advisory.

The parent module's `replace` directive applies this source to the service
and its SSO dependencies. Run `go test -race github.com/crewjam/saml` and
`go test -race ./internal/sso` from the parent module. Two spoofing cases fail
against the unmodified distribution. Remove the replacement once an upstream
release addresses the advisory and passes these regressions.
