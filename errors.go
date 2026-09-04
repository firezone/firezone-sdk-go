package firezone

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// APIError represents an RFC 9457 (Problem Details for HTTP APIs) error
// response from the Firezone API.
type APIError struct {
	// StatusCode is the HTTP status code of the response.
	StatusCode int
	// Type is the RFC 9457 problem type URI. The API always returns
	// "about:blank".
	Type string
	// Title is a short, human-readable summary of the problem, derived
	// from the HTTP status code (e.g. "Not Found").
	Title string
	// Detail is a human-readable explanation specific to this occurrence
	// of the problem.
	Detail string
	// ValidationErrors maps field names to validation failure messages.
	// Populated only when StatusCode is 422.
	ValidationErrors map[string][]string
	// RetryAfter is how long to wait before retrying, parsed from the
	// Retry-After response header. Populated only when StatusCode is 429.
	RetryAfter time.Duration
}

// Error implements the error interface.
func (e *APIError) Error() string {
	msg := fmt.Sprintf("firezone: %d %s", e.StatusCode, e.Title)
	if e.Detail != "" {
		msg += ": " + e.Detail
	}
	if len(e.ValidationErrors) > 0 {
		msg += " (" + formatValidationErrors(e.ValidationErrors) + ")"
	}
	return msg
}

func formatValidationErrors(errs map[string][]string) string {
	fields := make([]string, 0, len(errs))
	for field := range errs {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		parts = append(parts, fmt.Sprintf("%s: %s", field, strings.Join(errs[field], ", ")))
	}
	return strings.Join(parts, "; ")
}

// problemDetailsBody mirrors the JSON body of an RFC 9457 error response.
type problemDetailsBody struct {
	Type             string              `json:"type"`
	Title            string              `json:"title"`
	Status           int                 `json:"status"`
	Detail           string              `json:"detail"`
	ValidationErrors map[string][]string `json:"validation_errors"`
}

// parseAPIError builds an *APIError from a non-2xx HTTP response. The
// response body is assumed to be RFC 9457 problem+json; if it can't be
// parsed as such, the error still carries the status code and whatever
// detail can be salvaged.
func parseAPIError(resp *http.Response, body []byte) *APIError {
	apiErr := &APIError{
		StatusCode: resp.StatusCode,
		Title:      http.StatusText(resp.StatusCode),
	}

	var parsed problemDetailsBody
	if err := json.Unmarshal(body, &parsed); err == nil {
		apiErr.Type = parsed.Type
		if parsed.Title != "" {
			apiErr.Title = parsed.Title
		}
		apiErr.Detail = parsed.Detail
		apiErr.ValidationErrors = parsed.ValidationErrors
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		apiErr.RetryAfter = parseRetryAfter(resp.Header.Get("Retry-After"))
	}

	return apiErr
}

func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 0
	}
	var seconds int
	if _, err := fmt.Sscanf(header, "%d", &seconds); err != nil || seconds < 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

// IsNotFound reports whether err is an *APIError with StatusCode 404.
func IsNotFound(err error) bool { return hasStatus(err, http.StatusNotFound) }

// IsConflict reports whether err is an *APIError with StatusCode 409.
//
// The API returns 409 from a single endpoint - creating a token for a
// Gateway that already has one - which this SDK does not wrap, for the
// reasons in the [GatewaysService] doc comment. Most things that read
// like conflicts, a duplicate name among them, come back as 422
// validation errors instead, so reach for [IsValidation] first.
func IsConflict(err error) bool { return hasStatus(err, http.StatusConflict) }

// IsValidation reports whether err is an *APIError with StatusCode 422.
func IsValidation(err error) bool { return hasStatus(err, http.StatusUnprocessableEntity) }

// IsRateLimited reports whether err is an *APIError with StatusCode 429.
func IsRateLimited(err error) bool { return hasStatus(err, http.StatusTooManyRequests) }

// IsForbidden reports whether err is an *APIError with StatusCode 403.
func IsForbidden(err error) bool { return hasStatus(err, http.StatusForbidden) }

// IsUnauthorized reports whether err is an *APIError with StatusCode 401.
func IsUnauthorized(err error) bool { return hasStatus(err, http.StatusUnauthorized) }

func hasStatus(err error, status int) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode == status
}
