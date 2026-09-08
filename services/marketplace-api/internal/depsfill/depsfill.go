// Package depsfill populates the nil handler pointers on a Deps-shaped
// struct so a route registrar mounts EVERY route it can, including the ones
// behind an `if deps.XHandler != nil` guard.
//
// It exists so that route-shape guards — internal/handlers/admin's route
// parity test and internal/routemanifest's route manifest — enumerate the
// same, complete gin trees from one implementation. Two divergent copies of
// this reflection walk is its own defect: a fix to one would silently leave
// the other blind to a subset of routes, which is exactly the failure mode
// both guards exist to catch.
//
// It is only ever useful to a test: production wiring builds real handlers.
// It lives in a non-test file solely because more than one package's tests
// need it, and Go cannot share a helper defined in a _test.go file.
package depsfill

import (
	"reflect"
	"unsafe"

	"github.com/gin-gonic/gin"
)

// maxDepth bounds the recursion. Deps structs nest a couple of levels
// (Deps -> handler -> inner collaborator); three is enough for every
// registrar in this service and stops a self-referential type from
// looping forever.
const maxDepth = 3

// Struct fills the struct pointed to by ptr. Panics if ptr is not a
// non-nil pointer to a struct, because every caller passes a literal
// `&someDeps` and a silent no-op would make the caller's guard pass
// vacuously against an empty route table.
func Struct(ptr any) {
	v := reflect.ValueOf(ptr)
	if v.Kind() != reflect.Ptr || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		panic("depsfill.Struct: want a non-nil pointer to a struct, got " + v.Type().String())
	}
	fill(v.Elem(), 0)
}

// fill populates every nil handler pointer and gin.HandlerFunc on a
// Deps-shaped struct so that no `if deps.XHandler != nil` guard silently
// hides a route. It recurses into the handlers themselves — including
// unexported fields — because some registrars gate on inner state too
// (e.g. admin's RegisterAPIKeys returns early when APIKeysHandler.resolver
// is nil), and a half-populated handler would make a caller blind to four
// real routes.
//
// Nothing is invoked here: registration only takes method values, so zero
// structs are sufficient and no service, DB or FGA client is needed.
//
// Interface fields are deliberately NOT filled: reflection cannot invent an
// implementation. Callers must supply those themselves, and should assert
// none is left nil — see admin's assertNoNilDeps.
func fill(v reflect.Value, depth int) {
	if depth > maxDepth || v.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if !f.CanAddr() {
			continue
		}
		// Bypasses the unexported-field write barrier; safe because the value
		// is addressable and we only ever store freshly allocated zero values.
		w := reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
		switch f.Kind() {
		case reflect.Ptr:
			if f.Type().Elem().Kind() == reflect.Struct && w.IsNil() {
				allocated := reflect.New(f.Type().Elem())
				w.Set(allocated)
				fill(allocated.Elem(), depth+1)
			}
		case reflect.Struct:
			fill(w, depth+1)
		case reflect.Func:
			if f.Type() == reflect.TypeOf(gin.HandlerFunc(nil)) && w.IsNil() {
				w.Set(reflect.ValueOf(gin.HandlerFunc(func(c *gin.Context) { c.Next() })))
			}
		}
	}
}
