// Package routes owns platform-api's /internal route mounting.
//
// It exists so the route-to-guard mapping is testable (#323). It used to
// live inline in cmd/server/main.go, where nothing could assert it:
// moving a handler from the fail-closed group to the permissive one left
// `go build` and `go test ./...` green, so an estate-wide endpoint could
// be downgraded to a permissive guard and ship undetected.
package routes

import (
	"github.com/gin-gonic/gin"

	"github.com/mark8ly/platform-api/internal/middleware"
)

// Registrar mounts one handler's routes on a group.
type Registrar func(*gin.RouterGroup)

// InternalHandlers names every registrar mounted under /internal. A nil
// field is skipped — authHandler and the merchant account routes are
// conditional on configuration.
type InternalHandlers struct {
	TenantDirectory     Registrar
	TenantLifecycle     Registrar
	OnboardingAnalytics Registrar
	EstateCounts        Registrar
	EstateUsers         Registrar
	AccountOperator     Registrar

	Tenant          Registrar
	Store           Registrar
	Invitation      Registrar
	Auth            Registrar
	MerchantAccount Registrar
	Notification    Registrar
}

// StrictSlots and PermissiveSlots name which fields go behind which
// guard. They are the mapping in data form, so a test can assert it.
func StrictSlots() []string {
	return []string{
		"TenantDirectory", "TenantLifecycle", "OnboardingAnalytics",
		"EstateCounts", "EstateUsers", "AccountOperator",
	}
}

func PermissiveSlots() []string {
	return []string{
		"Tenant", "Store", "Invitation", "Auth", "MerchantAccount", "Notification",
	}
}

// MountInternal mounts both /internal groups.
//
// Two groups share the prefix deliberately. The permissive one no-ops on
// an empty secret, which keeps local dev working and makes the cutover
// safe; it is right for routes already scoped by something the caller had
// to know — you need a tenant id to ask for its members.
//
// The strict group refuses with 503 on an empty secret instead. Its
// routes return ESTATE-WIDE data — every tenant (#277), every staff
// identity (#278), platform-wide counts (#282), the onboarding funnel
// (#283) — so an unconfigured deploy must refuse rather than serve the
// lot to anything that reaches the pod.
//
// A nil registrar is skipped: authHandler and the merchant account routes
// are conditional on configuration.
func MountInternal(r gin.IRouter, secret string, h InternalHandlers) {
	strict := r.Group("/internal", middleware.RequireInternalAuthStrict(secret))
	mount(strict,
		h.TenantDirectory, h.TenantLifecycle, h.OnboardingAnalytics,
		h.EstateCounts, h.EstateUsers, h.AccountOperator,
	)

	permissive := r.Group("/internal", middleware.RequireInternalAuth(secret))
	mount(permissive,
		h.Tenant, h.Store, h.Invitation, h.Auth, h.MerchantAccount, h.Notification,
	)
}

func mount(g *gin.RouterGroup, regs ...Registrar) {
	for _, reg := range regs {
		if reg != nil {
			reg(g)
		}
	}
}
