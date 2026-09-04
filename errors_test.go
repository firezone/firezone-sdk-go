package firezone_test

import (
	"strings"
	"testing"

	firezone "github.com/firezone/firezone-sdk-go"
)

func TestAPIError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *firezone.APIError
		want string
	}{
		{
			name: "status and title only",
			err:  &firezone.APIError{StatusCode: 404, Title: "Not Found"},
			want: "firezone: 404 Not Found",
		},
		{
			name: "with detail",
			err:  &firezone.APIError{StatusCode: 409, Title: "Conflict", Detail: "name already taken"},
			want: "firezone: 409 Conflict: name already taken",
		},
		{
			name: "with validation errors",
			err: &firezone.APIError{
				StatusCode: 422,
				Title:      "Unprocessable Content",
				Detail:     "The request body failed validation.",
				ValidationErrors: map[string][]string{
					"name": {"can't be blank"},
				},
			},
			want: "firezone: 422 Unprocessable Content: The request body failed validation. (name: can't be blank)",
		},
		{
			name: "multiple validation errors are sorted by field",
			err: &firezone.APIError{
				StatusCode: 422,
				Title:      "Unprocessable Content",
				ValidationErrors: map[string][]string{
					"name":  {"can't be blank"},
					"email": {"is invalid", "has already been taken"},
				},
			},
			want: "firezone: 422 Unprocessable Content (email: is invalid, has already been taken; name: can't be blank)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAPIError_Error_ValidationErrorsMentionsEveryField(t *testing.T) {
	err := &firezone.APIError{
		StatusCode: 422,
		Title:      "Unprocessable Content",
		ValidationErrors: map[string][]string{
			"name":  {"can't be blank"},
			"email": {"is invalid"},
		},
	}

	msg := err.Error()
	for _, field := range []string{"name", "email"} {
		if !strings.Contains(msg, field) {
			t.Errorf("Error() = %q, want it to mention field %q", msg, field)
		}
	}
}
