package firezone_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	firezone "github.com/firezone/firezone-sdk-go"
	"github.com/firezone/firezone-sdk-go/internal/testutil"
)

// TestNewClientRejectsNilHTTPClient checks that a nil *http.Client is
// caught at construction. An Option can't report an error itself, so
// without this check the nil survives into the Client and the first
// request panics with a nil dereference, pointing at the request
// instead of at the option that caused it.
func TestNewClientRejectsNilHTTPClient(t *testing.T) {
	client, err := firezone.NewClient("https://api.example.com", "token",
		firezone.WithHTTPClient(nil))
	if err == nil {
		t.Fatalf("NewClient with a nil *http.Client returned no error, client = %v", client != nil)
	}
	if client != nil {
		t.Errorf("NewClient returned a non-nil client alongside an error")
	}
}

// TestWithRetryRejectsNegativeBudget is a regression test for a retry
// loop that made zero attempts.
//
// The loop runs `attempt <= maxRetries`, so a negative budget skipped
// the body entirely and fell through to a nil body with a nil error.
// That read to the caller as a successful request that returned
// nothing: no HTTP request was made, and the destination struct was
// left at its zero value with no error to say so. WithRetry now
// rejects the budget outright rather than guessing what was meant.
func TestWithRetryRejectsNegativeBudget(t *testing.T) {
	client, err := firezone.NewClient("https://api.example.com", "token",
		firezone.WithRetry(true, -1))
	if err == nil {
		t.Fatalf("NewClient with a negative retry budget returned no error, client = %v", client != nil)
	}
	if client != nil {
		t.Errorf("NewClient returned a non-nil client alongside an error")
	}
}

// TestWithUserAgentRejectsEmpty checks that an empty User-Agent is
// caught at construction. Setting the header to "" does not suppress
// it - net/http sends the empty value - so it is a mistake in every
// case, and omitting the option is how you keep the default.
func TestWithUserAgentRejectsEmpty(t *testing.T) {
	if _, err := firezone.NewClient("https://api.example.com", "token",
		firezone.WithUserAgent("")); err == nil {
		t.Fatal("NewClient with an empty User-Agent returned no error")
	}
}

// TestOptionErrorStopsConstruction checks that a failing option aborts
// NewClient rather than being applied and ignored.
func TestOptionErrorStopsConstruction(t *testing.T) {
	applied := false
	after := firezone.Option(func(*firezone.Client) error {
		applied = true
		return nil
	})

	if _, err := firezone.NewClient("https://api.example.com", "token",
		firezone.WithRetry(true, -1), after); err == nil {
		t.Fatal("NewClient returned no error")
	}
	if applied {
		t.Error("an option after the failing one was still applied")
	}
}

// TestNilRequestBody checks that a nil request struct is rejected
// before a request is made, for every method that takes one.
//
// A nil *CreateSiteRequest reaches wrapBody as an any holding a typed
// nil pointer, so a plain v == nil check reads false and the body
// encodes as {"site": null} - a request the API rejects for a reason
// that names nothing useful.
func TestNilRequestBody(t *testing.T) {
	const id = "00000000-0000-0000-0000-000000000000"

	client := testutil.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))

	ctx := context.Background()
	calls := map[string]func() error{
		"Sites.Create":     func() error { _, err := client.Sites.Create(ctx, nil); return err },
		"Sites.Update":     func() error { _, err := client.Sites.Update(ctx, id, nil); return err },
		"Resources.Create": func() error { _, err := client.Resources.Create(ctx, nil); return err },
		"Resources.Update": func() error { _, err := client.Resources.Update(ctx, id, nil); return err },
		"Policies.Create":  func() error { _, err := client.Policies.Create(ctx, nil); return err },
		"Policies.Update":  func() error { _, err := client.Policies.Update(ctx, id, nil); return err },
		"Groups.Create":    func() error { _, err := client.Groups.Create(ctx, nil); return err },
		"Groups.Update":    func() error { _, err := client.Groups.Update(ctx, id, nil); return err },
		"Actors.Create":    func() error { _, err := client.Actors.Create(ctx, nil); return err },
		"Actors.Update":    func() error { _, err := client.Actors.Update(ctx, id, nil); return err },
		"ClientDevices.Update": func() error {
			_, err := client.ClientDevices.Update(ctx, id, nil)
			return err
		},
		"Gateways.Provision": func() error {
			_, err := client.Sites.Gateways(id).Provision(ctx, nil)
			return err
		},
		"Gateways.Update": func() error {
			_, err := client.Sites.Gateways(id).Update(ctx, id, nil)
			return err
		},
	}

	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			err := call()
			if err == nil {
				t.Fatal("nil request body returned no error")
			}
			if !errors.Is(err, firezone.ErrNilRequest) {
				t.Errorf("error = %v, want one matching ErrNilRequest", err)
			}
		})
	}
}

// TestWithRequestTimeout checks that the SDK's own timeout bounds a
// request when the caller's context has no deadline of its own.
func TestWithRequestTimeout(t *testing.T) {
	release := make(chan struct{})
	client := testutil.NewClientWithOptions(t,
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }),
		firezone.WithRequestTimeout(100*time.Millisecond),
	)
	t.Cleanup(func() { close(release) })

	start := time.Now()
	_, err := client.Sites.Get(context.Background(), "site-id")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("a hung handler returned no error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want one matching context.DeadlineExceeded", err)
	}
	if elapsed > time.Second {
		t.Errorf("took %s; the timeout was not applied", elapsed)
	}
}

// TestWithRequestTimeoutZeroDisables checks that a zero timeout leaves
// the deadline entirely to the caller. The context below is what ends
// the request, not the SDK.
func TestWithRequestTimeoutZeroDisables(t *testing.T) {
	release := make(chan struct{})
	client := testutil.NewClientWithOptions(t,
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }),
		firezone.WithRequestTimeout(0),
	)
	t.Cleanup(func() { close(release) })

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if _, err := client.Sites.Get(ctx, "site-id"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want one matching context.DeadlineExceeded", err)
	}
}

// TestWithRequestTimeoutRejectsNegative checks that a negative duration
// is caught at construction. context.WithTimeout would read it as an
// already-expired deadline and fail every request before sending it.
func TestWithRequestTimeoutRejectsNegative(t *testing.T) {
	if _, err := firezone.NewClient("https://api.example.com", "token",
		firezone.WithRequestTimeout(-time.Second)); err == nil {
		t.Fatal("NewClient with a negative request timeout returned no error")
	}
}

// TestRequestTimeoutIsPerAttempt checks that the timeout bounds one
// attempt rather than a whole retried call: three attempts against a
// handler that stalls just under the timeout must all be made.
func TestRequestTimeoutIsPerAttempt(t *testing.T) {
	var attempts int
	client := testutil.NewClientWithOptions(t,
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			attempts++
			time.Sleep(20 * time.Millisecond)
			if attempts < 3 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"id":"site-id","name":"primary-dc"}}`))
		}),
		firezone.WithRequestTimeout(200*time.Millisecond),
		firezone.WithRetry(true, 5),
	)

	site, err := client.Sites.Get(context.Background(), "site-id")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
	if site.Name != "primary-dc" {
		t.Errorf("Name = %q, want %q", site.Name, "primary-dc")
	}
}
