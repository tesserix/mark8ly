package zitadellogin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// ErrIDPIntentInvalid is returned when Zitadel cannot find the idp intent
// being retrieved (a bad or already-consumed intent id/token pair, or a
// browser redirect that never happened).
var ErrIDPIntentInvalid = errors.New("zitadellogin: idp intent invalid")

// IDPIdentity is the federated identity Zitadel resolved for an idp intent.
//
// ZitadelUserID is the id of the EXISTING Zitadel user this intent is already
// linked to. It is empty when the intent has never been linked — that is the
// normal "first time this Google account has signed in" case, not an error,
// and callers must treat empty as "needs linking or registration", never as
// a malformed response.
//
// Email and EmailVerified are read from the provider's raw userinfo payload
// (rawInformation), NOT from a field Zitadel's own schema guarantees. Unlike
// the rest of this package's fields — which come from protojson and elide
// zero values by omission — this is the third-party IDP's own raw JSON
// (Google's OAuth2 userinfo shape: "email" / "email_verified"), a shape
// Zitadel merely passes through unmodified. This was not directly observed
// against a live retrieve response with a real Google account at the time
// this client was written; it is modelled on Google's documented userinfo
// response and Zitadel's documented pass-through of rawInformation. Treat it
// as best-effort: a provider that omits "email_verified" reads as
// EmailVerified=false here, which callers must not treat as an authoritative
// "unverified" from Zitadel itself.
type IDPIdentity struct {
	ZitadelUserID string
	Email         string
	EmailVerified bool

	// GivenName and FamilyName are read best-effort from the same raw
	// userinfo payload as Email/EmailVerified (see readRawName) — Google's
	// "given_name"/"family_name" claims, when the provider sends them.
	// Used ONLY as an optional profile hint for CreateHumanUserWithIDPLink's
	// brand-new-user profile; empty means "not sent", never an error, and
	// no caller makes a trust decision off either field the way it does
	// for Email/EmailVerified.
	GivenName  string
	FamilyName string

	// IDPID, ExternalUserID and ExternalUserName identify the federated
	// identity ITSELF — the Google IDP's own id on this Zitadel instance,
	// the external provider's user id (Google's stable "sub"), and the
	// display name/handle Zitadel recorded for it. These come from
	// idpInformation.{idpId,userId,userName}, not from rawInformation, so
	// (unlike Email/EmailVerified above) they are read from a field
	// Zitadel's own schema guarantees.
	//
	// Used ONLY to register a brand-new Zitadel user pre-linked to this
	// identity (see Client.CreateHumanUserWithIDPLink) when ZitadelUserID
	// is empty — a normal Zitadel sign-in never needs them, since the
	// session is created from the intent id/token, not from these values.
	IDPID            string
	ExternalUserID   string
	ExternalUserName string
}

// StartIDPIntent begins Zitadel's IDP-intent flow for the given idp (e.g. the
// Google IDP configured on this instance) and returns the URL the browser
// must be redirected to. There is no intent id in this response — Zitadel
// hands that back later on the successUrl/failureUrl redirect — so this
// function does not attempt to parse one out.
func (c *Client) StartIDPIntent(ctx context.Context, idpID, successURL, failureURL string) (string, error) {
	body := map[string]any{
		"idpId": idpID,
		"urls": map[string]any{
			"successUrl": successURL,
			"failureUrl": failureURL,
		},
	}
	var wire struct {
		AuthURL string `json:"authUrl"`
	}
	if err := c.do(ctx, http.MethodPost, "/v2/idp_intents", body, &wire, ErrUnavailable); err != nil {
		return "", err
	}
	if wire.AuthURL == "" {
		return "", fmt.Errorf("zitadellogin: start idp intent: 200 without an authUrl: %w", ErrUnavailable)
	}
	return wire.AuthURL, nil
}

// RetrieveIDPIntent exchanges the intent id and token carried on the browser
// redirect for the federated identity Zitadel resolved.
//
// The intent token is a bearer-equivalent secret for this one intent and
// must never be logged, exactly like a session token.
func (c *Client) RetrieveIDPIntent(ctx context.Context, intentID, intentToken string) (IDPIdentity, error) {
	body := map[string]any{
		"idpIntentToken": intentToken,
	}
	var wire struct {
		UserID         string `json:"userId"`
		IDPInformation struct {
			IDPID          string         `json:"idpId"`
			UserID         string         `json:"userId"`
			UserName       string         `json:"userName"`
			RawInformation map[string]any `json:"rawInformation"`
		} `json:"idpInformation"`
	}
	// withLogPath: the intent id is request-scoped input, not a secret the
	// way intentToken is, but there is no reason for it to ride along into
	// every error string this call can produce (which a caller may log) —
	// drop it from the logged path the same way the token is already kept
	// out of it entirely.
	if err := c.do(ctx, http.MethodPost, "/v2/idp_intents/"+url.PathEscape(intentID), body, &wire, ErrIDPIntentInvalid,
		withLogPath("/v2/idp_intents/{id}")); err != nil {
		return IDPIdentity{}, err
	}
	email, emailVerified := readRawEmail(wire.IDPInformation.RawInformation)
	givenName, familyName := readRawName(wire.IDPInformation.RawInformation)
	return IDPIdentity{
		ZitadelUserID:    wire.UserID,
		Email:            email,
		EmailVerified:    emailVerified,
		GivenName:        givenName,
		FamilyName:       familyName,
		IDPID:            wire.IDPInformation.IDPID,
		ExternalUserID:   wire.IDPInformation.UserID,
		ExternalUserName: wire.IDPInformation.UserName,
	}, nil
}

// readRawEmail pulls email/email_verified out of the provider's raw userinfo
// payload. It fails soft (empty email, unverified) on anything that is not
// exactly the shape it expects, rather than erroring the whole retrieve: this
// data is advisory (the handler is expected to re-derive trust decisions from
// it, not treat it as a Zitadel-guaranteed fact), and a provider that changes
// its raw payload shape must not break sign-in entirely.
func readRawEmail(raw map[string]any) (email string, verified bool) {
	if raw == nil {
		return "", false
	}
	if v, ok := raw["email"].(string); ok {
		email = v
	}
	if v, ok := raw["email_verified"].(bool); ok {
		verified = v
	}
	return email, verified
}

// readRawName pulls given_name/family_name out of the provider's raw
// userinfo payload the same best-effort way readRawEmail reads
// email/email_verified: empty when absent or not exactly a string, never an
// error. Used only as an optional profile hint (see IDPIdentity's doc) —
// never for a trust decision.
func readRawName(raw map[string]any) (givenName, familyName string) {
	if raw == nil {
		return "", ""
	}
	if v, ok := raw["given_name"].(string); ok {
		givenName = v
	}
	if v, ok := raw["family_name"].(string); ok {
		familyName = v
	}
	return givenName, familyName
}
