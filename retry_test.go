package firezone_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	firezone "github.com/firezone/firezone-sdk-go"
)

func TestRetry_SucceedsAfterRateLimitedResponses(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.Header().Set("Retry-After", "0")
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"type":"about:blank","title":"Too Many Requests","status":429,"detail":"slow down"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"id":"site-1","name":"primary-dc"}}`))
	}))
	defer server.Close()

	client, err := firezone.NewClient(server.URL, "test-token", firezone.WithRetry(true, 5))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	site, err := client.Sites.Get(ctx, "site-1")
	if err != nil {
		t.Fatalf("Get returned error after retries: %v", err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
	if site.ID != "site-1" {
		t.Errorf("site.ID = %q, want site-1", site.ID)
	}
}

func TestRetry_GivesUpAfterMaxRetries(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Retry-After", "0")
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"type":"about:blank","title":"Too Many Requests","status":429,"detail":"slow down"}`))
	}))
	defer server.Close()

	client, err := firezone.NewClient(server.URL, "test-token", firezone.WithRetry(true, 2))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.Sites.Get(context.Background(), "site-1")
	if !firezone.IsRateLimited(err) {
		t.Fatalf("IsRateLimited(err) = false, want true (err: %v)", err)
	}
	// maxRetries=2 means 3 total attempts (initial + 2 retries).
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestRetry_DisabledDoesNotRetry(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Retry-After", "0")
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"type":"about:blank","title":"Too Many Requests","status":429,"detail":"slow down"}`))
	}))
	defer server.Close()

	client, err := firezone.NewClient(server.URL, "test-token", firezone.WithRetry(false, 0))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = client.Sites.Get(context.Background(), "site-1")
	if !firezone.IsRateLimited(err) {
		t.Fatalf("IsRateLimited(err) = false, want true (err: %v)", err)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (retry disabled)", attempts)
	}
}

// TestRetry_HonorsRetryAfterAsAFloor checks that a wait is never
// shorter than the server asked for. Jitter is added on top of
// Retry-After, never subtracted from it - waiting less than the server
// requested guarantees another 429.
func TestRetry_HonorsRetryAfterAsAFloor(t *testing.T) {
	var attemptTimes []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptTimes = append(attemptTimes, time.Now())
		if len(attemptTimes) < 2 {
			w.Header().Set("Retry-After", "1")
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"type":"about:blank","title":"Too Many Requests","status":429}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"site-1","name":"primary-dc"}}`))
	}))
	defer server.Close()

	client, err := firezone.NewClient(server.URL, "test-token", firezone.WithRetry(true, 3))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if _, err := client.Sites.Get(context.Background(), "site-1"); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if len(attemptTimes) != 2 {
		t.Fatalf("attempts = %d, want 2", len(attemptTimes))
	}

	waited := attemptTimes[1].Sub(attemptTimes[0])
	if waited < time.Second {
		t.Errorf("waited %v between attempts, want at least the 1s Retry-After", waited)
	}
}

// TestRetry_JitterDesynchronizesConcurrentCallers is the property that
// matters for Terraform: at the default parallelism of 10, requests
// arrive together and are rate limited together. Without jitter every
// retry lands in the same instant and the same subset keeps winning,
// so the rest burn their whole budget. Distinct wait durations are what
// breaks that up.
func TestRetry_JitterDesynchronizesConcurrentCallers(t *testing.T) {
	const callers = 8

	var (
		mu    sync.Mutex
		waits = map[time.Duration]struct{}{}
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No Retry-After: exercises the exponential-backoff path, where
		// every caller would otherwise compute an identical wait.
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"type":"about:blank","title":"Too Many Requests","status":429}`))
	}))
	defer server.Close()

	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()

			client, err := firezone.NewClient(server.URL, "test-token", firezone.WithRetry(true, 1))
			if err != nil {
				t.Errorf("NewClient: %v", err)
				return
			}

			start := time.Now()
			_, _ = client.Sites.Get(context.Background(), "site-1")

			mu.Lock()
			defer mu.Unlock()
			// Round to 50ms so scheduling noise doesn't manufacture the
			// spread this test is looking for.
			waits[time.Since(start).Round(50*time.Millisecond)] = struct{}{}
		}()
	}
	wg.Wait()

	if len(waits) < 2 {
		t.Errorf("all %d callers waited the same rounded duration (%v); jitter is not being applied",
			callers, waits)
	}
}

// TestRetryWait_EscalatesAboveRetryAfter is the property this session's
// second round of 429s exposed: with Retry-After alone the wait never
// grew, so a request that kept losing the token race retried every
// second until its budget ran out. The header says when a token frees
// up, not who gets it.
func TestRetryWait_EscalatesAboveRetryAfter(t *testing.T) {
	client, err := firezone.NewClient("https://example.test", "token")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	const retryAfter = time.Second

	var previous time.Duration
	for attempt := range 6 {
		wait := firezone.ExportedRetryWait(client, attempt, retryAfter)

		if wait < retryAfter {
			t.Errorf("attempt %d waited %v, below the %v Retry-After floor", attempt, wait, retryAfter)
		}
		if attempt > 0 && wait <= previous {
			t.Errorf("attempt %d waited %v, not more than attempt %d's %v", attempt, wait, attempt-1, previous)
		}
		previous = wait
	}
}

func TestRetryWait_RespectsMaxWait(t *testing.T) {
	const cap = 5 * time.Second

	client, err := firezone.NewClient("https://example.test", "token",
		firezone.WithRetryMaxWait(cap))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	// Attempt 10 would be ~17 minutes uncapped. Jitter is added after
	// the cap, so allow for its ceiling on top.
	wait := firezone.ExportedRetryWait(client, 10, 0)
	if wait > cap+2*time.Second {
		t.Errorf("wait = %v, want no more than the %v cap plus jitter", wait, cap)
	}
	if wait < cap {
		t.Errorf("wait = %v, want at least the %v cap", wait, cap)
	}
}
