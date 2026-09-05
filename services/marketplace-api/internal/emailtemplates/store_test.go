package emailtemplates

import "testing"

func TestDecodeVariablesNeverReturnsNil(t *testing.T) {
	for name, raw := range map[string][]byte{
		"nil":     nil,
		"empty":   {},
		"null":    []byte("null"),
		"garbage": []byte("{not json"),
		"object":  []byte(`{"name":"x"}`),
	} {
		t.Run(name, func(t *testing.T) {
			// A nil slice marshals to null, and a console reading
			// variables.map(...) crashes on exactly the templates that
			// declare none.
			if got := decodeVariables(raw); got == nil {
				t.Fatal("decodeVariables returned nil")
			}
		})
	}
}

func TestDecodeVariablesDropsUnnamedAndDefaultsType(t *testing.T) {
	got := decodeVariables([]byte(`[{"name":"OrderNumber"},{"name":"  "},{"name":"Total","type":"number","required":true}]`))
	if len(got) != 2 {
		t.Fatalf("decodeVariables() = %v, want 2 entries (the unnamed one dropped)", got)
	}
	if got[0].Type != "string" {
		t.Errorf("missing type = %q, want the string default", got[0].Type)
	}
	if got[1].Type != "number" || !got[1].Required {
		t.Errorf("declared entry not preserved: %+v", got[1])
	}
}

// Every Store method must report a mis-wired process rather than panic on
// a nil *gorm.DB.
func TestStoreMethodsReportANilDatabase(t *testing.T) {
	s := NewStore(nil)
	if _, err := s.List(t.Context()); err != ErrNoDB {
		t.Errorf("List err = %v, want ErrNoDB", err)
	}
	if _, _, err := s.Get(t.Context(), "k"); err != ErrNoDB {
		t.Errorf("Get err = %v, want ErrNoDB", err)
	}
	if _, err := s.Upsert(t.Context(), UpsertInput{Key: "k"}); err != ErrNoDB {
		t.Errorf("Upsert err = %v, want ErrNoDB", err)
	}
}
