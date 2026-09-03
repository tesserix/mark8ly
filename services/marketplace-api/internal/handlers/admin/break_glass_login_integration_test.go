//go:build integration

package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/mark8ly/marketplace-api/internal/authbffclient"
	"github.com/mark8ly/marketplace-api/internal/breakglass"
	"github.com/mark8ly/marketplace-api/internal/handlers/admin"
	"github.com/mark8ly/marketplace-api/pkg/testdb"
)

// alwaysFailSecretClient simulates Secret Manager being completely
// unreachable — every call returns an error, regardless of path.
type alwaysFailSecretClient struct{}

func (alwaysFailSecretClient) AddVersion(context.Context, string, []byte) error {
	return errors.New("secret manager unreachable")
}

func (alwaysFailSecretClient) AccessLatest(context.Context, string) ([]byte, error) {
	return nil, errors.New("secret manager unreachable")
}

// provisionAccount seeds a real break-glass account (via the same
// Bootstrapper production uses) and returns its plaintext password and
// TOTP secret so a test can build a genuine login request.
func provisionAccount(t *testing.T, repo *breakglass.Repository, secrets *breakglass.SecretManager, tenantID uuid.UUID) (password, totpSecret string) {
	t.Helper()
	boot := breakglass.NewBootstrapper(repo, secrets, "test-project")
	require.NoError(t, boot.Provision(context.Background(), tenantID))

	acc, err := repo.GetByTenant(context.Background(), tenantID)
	require.NoError(t, err)
	blob, err := secrets.Fetch(context.Background(), acc.SecretPath)
	require.NoError(t, err)
	return blob.Password, blob.TOTPSecret
}

func newLoginRouter(deps admin.BreakGlassDeps) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := admin.NewBreakGlassLoginHandler(deps)
	r.POST("/admin/break-glass/login", h.Login)
	return r
}

func doLogin(t *testing.T, r *gin.Engine, tenantID uuid.UUID, password, totpCode string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{
		"tenant_id": tenantID.String(),
		"password":  password,
		"totp_code": totpCode,
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/admin/break-glass/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// The single most important test in this series (#404): a caller must not
// be able to tell a disabled account from a wrong-password attempt — not
// by status, not by body. Forensics lives in the audit log only.
func TestIntegration_Login_DisabledAccountIsByteIdenticalToWrongPassword(t *testing.T) {
	db := testdb.NewTx(t)
	repo := breakglass.NewRepository(db)
	secrets := breakglass.NewSecretManager(breakglass.NewFakeSecretClient())

	// The disabled tenant's password + TOTP are the REAL, CORRECT ones —
	// proving this is refused BECAUSE of disabled_at, not because the
	// credentials happen to be wrong too. A test that used a wrong
	// password here would still pass with no disable check at all.
	disabledTenant := uuid.New()
	disabledPassword, disabledTOTPSecret := provisionAccount(t, repo, secrets, disabledTenant)
	require.NoError(t, repo.Disable(context.Background(), disabledTenant, "compromised laptop"))
	disabledTOTPCode, err := breakglass.TOTPCode(disabledTOTPSecret, time.Now())
	require.NoError(t, err)

	wrongPwTenant := uuid.New()
	_, totpSecret := provisionAccount(t, repo, secrets, wrongPwTenant)
	validTOTP, err := breakglass.TOTPCode(totpSecret, time.Now())
	require.NoError(t, err)

	r := newLoginRouter(admin.BreakGlassDeps{
		Repo:        repo,
		Secrets:     secrets,
		RateLimiter: breakglass.NewLoginRateLimiter(),
		IPHMACKey:   breakglass.HMACKey("test-hmac-key"),
		Sessions:    authbffclient.NoopIssuer{},
	})

	disabledResp := doLogin(t, r, disabledTenant, disabledPassword, disabledTOTPCode)
	wrongPwResp := doLogin(t, r, wrongPwTenant, "definitely-the-wrong-password", validTOTP)

	require.Equal(t, http.StatusUnauthorized, disabledResp.Code)
	require.Equal(t, http.StatusUnauthorized, wrongPwResp.Code)
	require.Equal(t, wrongPwResp.Body.String(), disabledResp.Body.String(),
		"a disabled account's failure response must be BYTE-IDENTICAL to a wrong-password failure")
	require.JSONEq(t, `{"error":"invalid_credentials"}`, disabledResp.Body.String())
}

// The disable check must fire even when Secret Manager is completely
// unreachable — proving it short-circuits BEFORE the fetch (step 4), not
// merely produces the same eventual status by another route.
func TestIntegration_Login_DisabledAccountShortCircuitsBeforeSecretFetch(t *testing.T) {
	db := testdb.NewTx(t)
	repo := breakglass.NewRepository(db)
	// Seed with a WORKING secret manager so Provision succeeds...
	workingSecrets := breakglass.NewSecretManager(breakglass.NewFakeSecretClient())
	tenantID := uuid.New()
	_, _ = provisionAccount(t, repo, workingSecrets, tenantID)
	require.NoError(t, repo.Disable(context.Background(), tenantID, "incident #999"))

	// ...but wire the HANDLER to a secret manager that always fails, so
	// any code path that reaches step 4 would surface as a 500, not a 401.
	brokenSecrets := breakglass.NewSecretManager(alwaysFailSecretClient{})

	r := newLoginRouter(admin.BreakGlassDeps{
		Repo:        repo,
		Secrets:     brokenSecrets,
		RateLimiter: breakglass.NewLoginRateLimiter(),
		IPHMACKey:   breakglass.HMACKey("test-hmac-key"),
		Sessions:    authbffclient.NoopIssuer{},
	})

	resp := doLogin(t, r, tenantID, "irrelevant", "000000")

	require.Equal(t, http.StatusUnauthorized, resp.Code,
		"a disabled account must be refused before Secret Manager is ever touched — "+
			"a 500 here would mean the check ran AFTER the fetch")
	require.JSONEq(t, `{"error":"invalid_credentials"}`, resp.Body.String())
}

// Enable must restore login for an account that was previously disabled.
func TestIntegration_Login_Enable_RestoresLogin(t *testing.T) {
	db := testdb.NewTx(t)
	repo := breakglass.NewRepository(db)
	secrets := breakglass.NewSecretManager(breakglass.NewFakeSecretClient())

	tenantID := uuid.New()
	password, totpSecret := provisionAccount(t, repo, secrets, tenantID)
	require.NoError(t, repo.Disable(context.Background(), tenantID, "testing disable"))

	r := newLoginRouter(admin.BreakGlassDeps{
		Repo:        repo,
		Secrets:     secrets,
		RateLimiter: breakglass.NewLoginRateLimiter(),
		IPHMACKey:   breakglass.HMACKey("test-hmac-key"),
		Sessions:    authbffclient.StaticIssuer{},
	})

	code, err := breakglass.TOTPCode(totpSecret, time.Now())
	require.NoError(t, err)

	disabledResp := doLogin(t, r, tenantID, password, code)
	require.Equal(t, http.StatusUnauthorized, disabledResp.Code, "must be refused while disabled")

	require.NoError(t, repo.Enable(context.Background(), tenantID))

	// A fresh TOTP code: enough wall-clock may have passed between the two
	// requests to cross a 30s window boundary.
	code2, err := breakglass.TOTPCode(totpSecret, time.Now())
	require.NoError(t, err)
	enabledResp := doLogin(t, r, tenantID, password, code2)
	require.Equal(t, http.StatusOK, enabledResp.Code, "Enable must restore login")
}
