// Package testutil provides shared httptest helpers for table-driven
// api-client tests.
package testutil

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	firezone "github.com/firezone/firezone-sdk-go"
)

// NewClient starts an httptest.Server serving handler and returns a
// *firezone.Client pointed at it, with retries disabled (tests assert
// on individual requests, not retry behavior - see RetryClient for
// that). The server is closed automatically via t.Cleanup.
func NewClient(t *testing.T, handler http.Handler) *firezone.Client {
	t.Helper()
	return NewClientWithOptions(t, handler)
}

// NewClientWithOptions is NewClient with extra options appended, for
// tests that need to configure the client under test. Retries stay
// disabled unless an option re-enables them.
func NewClientWithOptions(t *testing.T, handler http.Handler, opts ...firezone.Option) *firezone.Client {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client, err := firezone.NewClient(server.URL, "test-token",
		append([]firezone.Option{firezone.WithRetry(false, 0)}, opts...)...)
	if err != nil {
		t.Fatalf("testutil: building client: %v", err)
	}
	return client
}

// JSONResponse writes v as a JSON response body with the given status
// code, matching the API's {"data": ...} or {"data": ..., "metadata": ...}
// envelope shapes - callers pass the full envelope, not just the payload.
func JSONResponse(status int, v any) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if err := json.NewEncoder(w).Encode(v); err != nil {
			panic(fmt.Sprintf("testutil: encoding response: %v", err))
		}
	}
}

// ProblemResponse writes an RFC 9457 problem+json error response
// matching the API's PortalAPI.ProblemDetails.send/4 shape.
func ProblemResponse(status int, detail string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		JSONResponse(status, map[string]any{
			"type":   "about:blank",
			"title":  http.StatusText(status),
			"status": status,
			"detail": detail,
		})(w, r)
	}
}

// ValidationErrorResponse writes a 422 problem+json response with the
// given field->messages validation errors.
func ValidationErrorResponse(fields map[string][]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		JSONResponse(http.StatusUnprocessableEntity, map[string]any{
			"type":              "about:blank",
			"title":             http.StatusText(http.StatusUnprocessableEntity),
			"status":            http.StatusUnprocessableEntity,
			"detail":            "The request body failed validation.",
			"validation_errors": fields,
		})(w, r)
	}
}

// RateLimitedResponse writes a 429 problem+json response with a
// Retry-After header, matching the API's rate limit plug.
func RateLimitedResponse(retryAfterSeconds int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfterSeconds))
		ProblemResponse(http.StatusTooManyRequests, "Rate limit exceeded.")(w, r)
	}
}
