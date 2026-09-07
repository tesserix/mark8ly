package zitadeladmin

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/mark8ly/platform-api/internal/idperr"
)

// policyBody builds the error envelope POST /v2/users/human returns for a
// complexity rejection, in the shape probed live on 2026-09-05.
func policyBody(id string) string {
	return `{"code":3,"message":"Errors.User.PasswordComplexityPolicy.NotSatisfied","details":[{"@type":"...","id":"` + id + `"}]}`
}

// TestPasswordPolicy_EachIDMapsToItsOwnRule is the core of the fix: all
// five live ids must resolve to DIFFERENT rules and DIFFERENT messages.
// Collapsing any two would put the merchant back where the incident
// started — told to change something that was not what they got wrong.
func TestPasswordPolicy_EachIDMapsToItsOwnRule(t *testing.T) {
	cases := []struct {
		id       string
		wantRule string
		sentinel error
		// wantIn is a fragment the message must name, so a future edit
		// cannot quietly swap two rows' text.
		wantIn string
	}{
		{"COMMA-HuJf6", RuleTooShort, ErrPasswordTooShort, "too short"},
		{"COMMA-VoaRj", RuleNoUppercase, ErrPasswordNoUppercase, "uppercase"},
		{"COMMA-co3Xw", RuleNoLowercase, ErrPasswordNoLowercase, "lowercase"},
		{"COMMA-ZBv4H", RuleNoNumber, ErrPasswordNoNumber, "number"},
		{"COMMA-ZDLwA", RuleNoSymbol, ErrPasswordNoSymbol, "symbol"},
	}

	seenRules := map[string]string{}
	seenMessages := map[string]string{}

	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(policyBody(tc.id)))
			})
			_, err := c.EnsureHumanUser(context.Background(), HumanUser{
				Email: "invitee@example.com", FirstName: "In", LastName: "Vitee", Password: "Test@123_01",
			})
			if err == nil {
				t.Fatal("EnsureHumanUser = nil error, want a password policy rejection")
			}
			if !errors.Is(err, tc.sentinel) {
				t.Fatalf("err = %v, want sentinel %v", err, tc.sentinel)
			}
			// The coarse sentinel must still match, or internal/auth's
			// existing ErrWeakPassword branches stop firing.
			if !errors.Is(err, idperr.ErrWeakPassword) {
				t.Errorf("err = %v, must also match idperr.ErrWeakPassword", err)
			}

			var v interface {
				PasswordPolicyRule() (string, string)
			}
			if !errors.As(err, &v) {
				t.Fatalf("err = %v, want it to expose PasswordPolicyRule", err)
			}
			rule, message := v.PasswordPolicyRule()
			if rule != tc.wantRule {
				t.Errorf("rule = %q, want %q", rule, tc.wantRule)
			}
			if !strings.Contains(strings.ToLower(message), tc.wantIn) {
				t.Errorf("message = %q, want it to name %q", message, tc.wantIn)
			}
			if prev, ok := seenRules[rule]; ok {
				t.Errorf("rule %q already produced by %s — ids must not collapse", rule, prev)
			}
			seenRules[rule] = tc.id
			if prev, ok := seenMessages[message]; ok {
				t.Errorf("message already produced by %s — each rule needs its own text", prev)
			}
			seenMessages[message] = tc.id

			// The classified error must never carry the password.
			if strings.Contains(err.Error(), "Test@123_01") {
				t.Errorf("err.Error() = %q contains the password", err.Error())
			}
		})
	}
}

// TestPasswordPolicy_SuffixCollisionDoesNotCrossMap is the discipline
// this package's docs already demand, applied to the new table:
// COMMA-HuJf6 and DOMAIN-HuJf6 share a five-character suffix and are
// different errors from different Zitadel commands. Matching on the
// suffix would make either one impersonate the other.
func TestPasswordPolicy_SuffixCollisionDoesNotCrossMap(t *testing.T) {
	t.Run("DOMAIN-HuJf6 is not classified as a COMMA rule", func(t *testing.T) {
		if _, ok := zitadelPasswordPolicyIDs["DOMAIN-HuJf6"]; ok {
			t.Fatal("DOMAIN-HuJf6 must not be in the COMMA-keyed policy table")
		}
		c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":3,"message":"Password is too short","details":[{"id":"DOMAIN-HuJf6"}]}`))
		})
		_, err := c.EnsureHumanUser(context.Background(), HumanUser{
			Email: "a@example.com", FirstName: "A", LastName: "B", Password: "short",
		})
		var v interface {
			PasswordPolicyRule() (string, string)
		}
		if errors.As(err, &v) {
			rule, _ := v.PasswordPolicyRule()
			t.Fatalf("DOMAIN-HuJf6 was classified as rule %q via suffix matching", rule)
		}
		// It keeps its long-standing coarse meaning.
		if !errors.Is(err, idperr.ErrWeakPassword) {
			t.Errorf("err = %v, want idperr.ErrWeakPassword", err)
		}
	})

	t.Run("COMMA-HuJf6 does not answer to the DOMAIN sentinel path", func(t *testing.T) {
		// Both map to ErrWeakPassword by design; what must differ is that
		// only the COMMA id carries a rule classification.
		got := asPasswordPolicyError(&apiError{id: "DOMAIN-HuJf6", sentinel: idperr.ErrWeakPassword})
		var v interface {
			PasswordPolicyRule() (string, string)
		}
		if errors.As(got, &v) {
			t.Fatal("asPasswordPolicyError classified DOMAIN-HuJf6")
		}
		got = asPasswordPolicyError(&apiError{id: "COMMA-HuJf6", sentinel: idperr.ErrWeakPassword})
		if !errors.As(got, &v) {
			t.Fatal("asPasswordPolicyError did not classify COMMA-HuJf6")
		}
	})
}

// TestPasswordPolicy_UnrelatedErrorsPassThroughUnchanged pins that the
// new wrapper is inert for everything else — an already-exists 409 in
// particular still resolves to the existing user rather than being
// converted into a user-facing password complaint.
func TestPasswordPolicy_UnrelatedErrorsPassThroughUnchanged(t *testing.T) {
	for _, id := range []string{"", "V3-DKcYh", "COMMAND-SAF4f", "COMMA-notreal"} {
		in := &apiError{id: id, sentinel: idperr.ErrUnavailable}
		if got := asPasswordPolicyError(in); got != error(in) {
			t.Errorf("asPasswordPolicyError(id=%q) rewrapped an unrelated error: %v", id, got)
		}
	}
	if asPasswordPolicyError(nil) != nil {
		t.Error("asPasswordPolicyError(nil) must stay nil")
	}
	plain := errors.New("dial tcp: connection refused")
	if got := asPasswordPolicyError(plain); got != plain {
		t.Errorf("asPasswordPolicyError rewrapped a non-Zitadel error: %v", got)
	}
}

// TestPasswordPolicy_MinLengthMatchesLivePolicy guards the copied
// constant against drifting away from the message that quotes it.
func TestPasswordPolicy_MinLengthMatchesLivePolicy(t *testing.T) {
	if PasswordMinLength != 12 {
		t.Fatalf("PasswordMinLength = %d, want 12 (live org policy minLength, probed 2026-09-05)", PasswordMinLength)
	}
	_, msg := (&policyError{rule: zitadelPasswordPolicyIDs["COMMA-HuJf6"]}).PasswordPolicyRule()
	if !strings.Contains(msg, "12") {
		t.Errorf("too-short message %q does not quote the minimum length", msg)
	}
}
