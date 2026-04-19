package apple

import (
	"bytes"
	"encoding/json"
	"io"
)

// jsonReader wraps bytes.Buffer but keeps the io.Reader typed so
// http.NewRequestWithContext accepts it directly (it rejects a typed
// *bytes.Buffer with a non-nil check on untyped nil otherwise).
type jsonReader struct{ io.Reader }

func newJSONReader(body any) (*jsonReader, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return &jsonReader{Reader: bytes.NewReader(b)}, nil
}
