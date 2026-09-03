// Command server is the auth-bff HTTP entrypoint.
//
// Wires GIP verifier + OpenFGA client + session manager + autologin into
// the HTTP server. The session manager and OpenFGA store ID are constructed
// at startup; the schema version is asserted before the server binds.
package main

import (
	"context"
	"database/sql"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	authbff "github.com/mark8ly/auth-bff"
	"github.com/mark8ly/auth-bff/internal/adminhandoff"
	"github.com/mark8ly/auth-bff/internal/audit"
	"github.com/mark8ly/auth-bff/internal/authz"
	"github.com/mark8ly/auth-bff/internal/autologin"
	"github.com/mark8ly/auth-bff/internal/deviceguard"
	"github.com/mark8ly/auth-bff/internal/emailotp"
	"github.com/mark8ly/auth-bff/internal/gip"
	"github.com/mark8ly/auth-bff/internal/loginotp"
	"github.com/mark8ly/auth-bff/internal/notify"
	"github.com/mark8ly/auth-bff/internal/observability"
	"github.com/mark8ly/auth-bff/internal/session"
	"github.com/mark8ly/auth-bff/internal/usermfa"
	"github.com/mark8ly/auth-bff/internal/usersessions"
	"github.com/mark8ly/auth-bff/internal/zitadellogin"
	"github.com/mark8ly/auth-bff/pkg/config"
	"github.com/mark8ly/auth-bff/pkg/httpserver"
	"github.com/mark8ly/auth-bff/pkg/logger"
	"github.com/mark8ly/auth-bff/pkg/migrate"
)

// serviceName is the OpenTelemetry service.name attribute and the label
// used by the gin OTel middleware.
const serviceName = "mark8ly-auth-bff"

// notifierOrNil returns a genuinely nil interface for a nil client, so
// deviceguard's `notifier != nil` check means what it reads as rather
// than holding a typed nil.
func notifierOrNil(c *notify.Client) deviceguard.Notifier {
	if c == nil {
		return nil
	}
	return c
}

// registryAdapter bridges the session registry onto the narrower
// interface loginotp declares for its own use.
type registryAdapter struct{ repo *usersessions.Repository }

func (a registryAdapter) CreateSession(ctx context.Context, p loginotp.CreateParams) error {
	_, err := a.repo.Create(ctx, usersessions.CreateParams{
		UserID:      p.UserID,
		TenantID:    p.TenantID,
		Device:      p.Device,
		IPAddress:   p.IPAddress,
		UserAgent:   p.UserAgent,
		Fingerprint: p.Fingerprint,
	})
	return err
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	log := logger.New(cfg.Env)

	// ─── OpenTelemetry (traces + metrics) ──────────────────────────────
	// No-op when OTEL_EXPORTER_OTLP_ENDPOINT is empty. Init early so the
	// global providers are installed before any instrumented startup work.
	otelShutdown, err := observability.Init(context.Background(), serviceName)
	if err != nil {
		log.Warn("otel: init failed — continuing without telemetry", "err", err)
	} else {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := otelShutdown(shutdownCtx); err != nil {
				log.Warn("otel: shutdown", "err", err)
			}
		}()
	}

	// ─── Schema version gate ───────────────────────────────────────────
	mig, err := migrate.New(authbff.MigrationsFS, "migrations", cfg.DatabaseURL)
	if err != nil {
		log.Error("migrate init", "err", err)
		panic(err)
	}
	if err := mig.AssertVersion(authbff.ExpectedSchemaVersion); err != nil {
		log.Error("schema version mismatch — run migrations first", "err", err)
		panic(err)
	}

	// ─── DB connection for the user_sessions registry ──────────────────
	// database/sql + lib/pq — lightweight, no ORM. The auth flow does
	// not need one; the registry just needs simple insert/select.
	dbConn, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		log.Error("db: open", "err", err)
		panic(err)
	}
	dbConn.SetMaxOpenConns(5)
	dbConn.SetMaxIdleConns(2)
	dbConn.SetConnMaxLifetime(30 * time.Minute)
	if err := dbConn.Ping(); err != nil {
		log.Error("db: ping", "err", err)
		panic(err)
	}
	defer dbConn.Close()

	sessionRegistry := usersessions.NewRepository(dbConn)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ─── GIP verifier ──────────────────────────────────────────────────
	verifier, err := gip.New(ctx, gip.Config{
		ProjectID: cfg.GIPProjectID,
	})
	if err != nil {
		log.Error("gip: new verifier", "err", err)
		panic(err)
	}

	// ─── OpenFGA client (store resolved lazily) ────────────────────────
	// Resolution is deferred to first use so a boot that races openfga's
	// startup degrades to 503-and-retry instead of killing authorization
	// for the life of the pod.
	fgaClient := authz.NewLazy(authz.LazyConfig{
		APIURL:  cfg.FGAAPIURL,
		StoreID: cfg.FGAStoreID,
		Logger:  log,
	})

	// ─── Session manager ───────────────────────────────────────────────
	// Secure flag: only set in non-dev. Plain HTTP localhost dev needs
	// Secure=false or the browser silently drops the cookie.
	sessions, err := session.NewManager(session.Config{
		CookieName: cfg.SessionCookieName,
		Domain:     cfg.SessionCookieDomain,
		Secure:     cfg.Env != "dev",
		EncryptKey: cfg.SessionEncryptKey,
	})
	if err != nil {
		log.Error("session: new manager", "err", err)
		panic(err)
	}

	// ─── MFA (TOTP) ────────────────────────────────────────────────────
	// Constructed before autologin so the MFA gate can be injected into
	// the login path. Reuses the session encrypt key for at-rest secret
	// encryption — fatal-on-init-error because MFA cannot degrade to
	// plaintext storage safely.
	mfaSvc, err := usermfa.NewService(dbConn, cfg.SessionEncryptKey)
	if err != nil {
		log.Error("usermfa: new service", "err", err)
		panic(err)
	}
	mfaHandler := usermfa.NewHandler(mfaSvc, log)

	// ─── Cross-service audit client ────────────────────────────────────
	// Posts user.signed_in / user.signed_out events to marketplace-api's
	// /internal/audit-events ingest endpoint. Empty MarketplaceAPIURL
	// disables it so dev environments without marketplace-api still boot.
	auditClient := audit.New(cfg.MarketplaceAPIURL, cfg.AuditIngestSecret, log)

	// ─── New-device detection + email OTP step-up ──────────────────────
	// notify posts to platform-api's /internal/notifications/send. With
	// no PLATFORM_API_URL the client errors on every call, so deviceguard
	// is only given a notifier when it can actually deliver.
	var notifier *notify.Client
	if cfg.PlatformAPIURL != "" {
		notifier = notify.New(notify.Config{
			BaseURL:      cfg.PlatformAPIURL,
			AuthSecret:   cfg.PlatformAPIInternalSecret,
			SupportEmail: cfg.NotificationSupportEmail,
			SecureURL:    cfg.NotificationSecurityURL,
		})
	} else {
		log.Warn("notify: PLATFORM_API_URL unset — new-device alerts and email OTP are disabled")
	}

	var deviceEvaluator autologin.DeviceEvaluator
	deviceSvc, err := deviceguard.NewService(deviceguard.Config{
		Store:    deviceguard.NewSessionStore(dbConn),
		Notifier: notifierOrNil(notifier),
		Logger:   log,
	})
	if err != nil {
		log.Error("deviceguard: new service", "err", err)
		panic(err)
	}
	deviceEvaluator = deviceSvc

	// The gate is wired only when both halves exist: a pepper to hash
	// codes with and a way to mail them. A half-configured gate would
	// block logins it cannot complete.
	var otpIssuer autologin.ChallengeIssuer
	var otpHandler *loginotp.Handler
	if cfg.EmailOTPPepper != "" && notifier != nil {
		otpSvc, err := emailotp.NewService(emailotp.Config{
			Store:  emailotp.NewPostgresStore(dbConn),
			Pepper: cfg.EmailOTPPepper,
		})
		if err != nil {
			log.Error("emailotp: new service", "err", err)
			panic(err)
		}
		gate := loginotp.NewGate(otpSvc, notifier, emailotp.DefaultTTL)
		otpIssuer = gate
		otpHandler = loginotp.NewHandler(loginotp.Config{
			Gate:     gate,
			Sessions: sessions,
			Registry: registryAdapter{sessionRegistry},
			Logger:   log,
		})
		log.Info("emailotp: new-device sign-in challenge enabled")
	} else {
		log.Warn("emailotp: disabled — set EMAIL_OTP_PEPPER and PLATFORM_API_URL to gate unrecognised devices")
	}

	// ─── Autologin ─────────────────────────────────────────────────────
	autologinSvc := autologin.NewService(autologin.Config{
		GIP:      verifier,
		FGA:      fgaClient,
		Sessions: sessions,
		Registry: sessionRegistry,
		MFA:      mfaSvc,
		Devices:  deviceEvaluator,
		EmailOTP: otpIssuer,
		Audit:    auditClient,
		Logger:   log,
	})
	autologinHandler := autologin.NewHandler(autologinSvc)

	// ─── Zitadel login client (#524 phase 2) ────────────────────────────
	// Constructed only when explicitly enabled AND fully configured; nil
	// otherwise, so no route it backs is mounted. Refusing to boot on
	// partial config is deliberate: a half-configured login path that fails
	// at request time is worse than a loud failure here.
	var zitadelClient *zitadellogin.Client
	switch {
	case !cfg.ZitadelEnabled:
		log.Info("zitadel login disabled; GIP remains the auth provider")
	case cfg.ZitadelIssuer == "" || cfg.ZitadelLoginClientToken == "":
		log.Error("zitadel: ZITADEL_ENABLED is set but ZITADEL_ISSUER or ZITADEL_LOGIN_CLIENT_TOKEN is empty")
		panic("zitadel: enabled but not configured")
	default:
		zitadelClient = zitadellogin.New(cfg.ZitadelIssuer, cfg.ZitadelLoginClientToken, nil)
		log.Info("zitadel login client enabled", "issuer", cfg.ZitadelIssuer)
	}
	// Task 5 (wiring) will use zitadelClient to mount routes.
	_ = zitadelClient

	// ─── Session introspection + logout ────────────────────────────────
	sessionHandler := session.NewHandler(sessions, fgaClient).
		WithRegistry(sessionRegistry, log).
		WithMFA(mfaSvc).
		WithAudit(auditClient).
		WithGIPLookup(cfg.GIPWebAPIKey, cfg.GIPInternalTenantID)

	// ─── Cross-TLD admin handoff ───────────────────────────────────────
	// Mints a session cookie scoped to a custom admin domain
	// (admin.<merchant-tld>) given a short-lived HMAC-signed handoff
	// code from the canonical admin app. The receiving handler
	// (apps/admin/app/auth/handoff/route.ts) POSTs the code here after
	// verifying it locally, and forwards the resulting Set-Cookie to
	// the browser scoped to the custom domain.
	adminHandoffHandler := adminhandoff.NewHandler(adminhandoff.Config{
		HMACKey:  cfg.SessionEncryptKey,
		FGA:      fgaClient,
		Sessions: sessions,
		Logger:   log,
	})

	// ─── HTTP routes ───────────────────────────────────────────────────
	r := httpserver.New(cfg.Env, log)
	// OTel HTTP server instrumentation: one span per request, propagated
	// trace context. No-op spans when the global provider is disabled.
	r.Use(otelgin.Middleware(serviceName))
	v1 := r.Group("/auth")
	autologinHandler.Register(v1)
	sessionHandler.Register(v1)
	adminHandoffHandler.Register(v1)
	if otpHandler != nil {
		otpHandler.Register(v1)
	}

	// /api/v1 surface consumed by marketplace-api's account handler,
	// which proxies admin UI requests through to us. Kept separate from
	// /auth so the existing login + cookie routes stay untouched.
	apiV1 := r.Group("/api/v1")
	sessionHandler.RegisterAPI(apiV1)
	mfaHandler.Register(apiV1)

	// /internal surface for service-to-service calls. Guarded per-route
	// by the X-Internal-Auth header; the handler fails closed when the
	// secret is unset. marketplace-api calls DELETE /internal/users/:id
	// when the merchant triggers "reset my profile".
	internalGroup := r.Group("/internal")
	internalUsers := session.NewInternalUsersHandler(
		sessionRegistry,
		mfaSvc,
		cfg.MarketplaceInternalAuthSecret,
		log,
	)
	internalUsers.Register(internalGroup)

	if err := httpserver.Run(ctx, cfg.HTTPPort, r, log); err != nil {
		log.Error("http server", "err", err)
		panic(err)
	}
}
