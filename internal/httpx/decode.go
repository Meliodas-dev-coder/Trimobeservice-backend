package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// maxJSONBody caps the size of a JSON request body (1 MiB). JSON API payloads
// are small; this bounds memory use and blocks oversized-body DoS attempts.
const maxJSONBody = 1 << 20

// DecodeJSON strictly decodes a single JSON object from the request body into
// dst, rejecting unknown fields and trailing data. The body is size-limited. It
// returns a user-safe error message on failure.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var syntaxErr *json.SyntaxError
		var typeErr *json.UnmarshalTypeError
		var maxErr *http.MaxBytesError
		switch {
		case errors.As(err, &maxErr):
			return fmt.Errorf("request body must not exceed %d bytes", maxErr.Limit)
		case errors.As(err, &syntaxErr):
			return fmt.Errorf("body contains malformed JSON (at position %d)", syntaxErr.Offset)
		case errors.Is(err, io.ErrUnexpectedEOF):
			return errors.New("body contains malformed JSON")
		case errors.As(err, &typeErr):
			return fmt.Errorf("invalid value for field %q", typeErr.Field)
		case errors.Is(err, io.EOF):
			return errors.New("body must not be empty")
		default:
			return err
		}
	}

	// Ensure there is exactly one JSON value in the body.
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("body must contain a single JSON object")
	}
	return nil
}
