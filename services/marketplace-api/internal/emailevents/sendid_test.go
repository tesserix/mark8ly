package emailevents

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// Resend has echoed tags back both as an object and as a list of
// {name, value}. Reading only the object shape dropped every list-shaped
// event, so delivery and bounce were never recorded.
func TestSendIDFrom_EveryTagShape(t *testing.T) {
	id := uuid.New()
	for name, data := range map[string]string{
		"object":      `{"email_id":"e1","tags":{"kind":"receipt","send_id":"` + id.String() + `"}}`,
		"list":        `{"email_id":"e1","tags":[{"name":"kind","value":"receipt"},{"name":"send_id","value":"` + id.String() + `"}]}`,
		"custom_args": `{"custom_args":{"send_id":"` + id.String() + `"}}`,
	} {
		got, ok := sendIDFrom(json.RawMessage(data))
		if !ok || got != id {
			t.Errorf("%s: got %v %v", name, got, ok)
		}
	}
	// A list-shaped tags field must not stop the other places being read.
	if got, ok := sendIDFrom(json.RawMessage(`{"tags":[{"name":"kind","value":"x"}],"headers":{"send_id":"` + id.String() + `"}}`)); !ok || got != id {
		t.Errorf("headers after list tags: %v %v", got, ok)
	}
	if _, ok := sendIDFrom(json.RawMessage(`{"tags":[{"name":"kind","value":"invite"}]}`)); ok {
		t.Error("an email we did not log has no send id")
	}
}
