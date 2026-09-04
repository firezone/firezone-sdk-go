package firezone

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"net/url"
	"time"
)

// defaultMaxRetryWait caps how long a single retry will wait. Without a
// cap the exponential curve reaches minutes, long past the point where
// a caller would rather see the error. Override with [WithRetryMaxWait].
const defaultMaxRetryWait = 30 * time.Second

// maxRetryJitter caps the random padding added to every wait. The
// padding exists to break up thundering herds: Terraform applies with
// the default parallelism of 10 send bursts of concurrent requests, and
// without jitter every one of them is rate limited at the same instant,
// waits the same duration, and retries in lockstep - so the same subset
// keeps winning and the rest exhaust their retries.
const maxRetryJitter = 2 * time.Second

// requestWithRetry calls rawRequest, retrying on HTTP 429 (rate
// limited) responses. Retrying is a request-level concern (not a
// http.RoundTripper-level one) because it needs structured access to
// APIError.RetryAfter, which a RoundTripper wrapper would have to
// re-parse the body to get.
func (c *Client) requestWithRetry(ctx context.Context, method, requestPath string, query url.Values, body requestBody) ([]byte, error) {
	if !c.retryEnabled {
		return c.rawRequest(ctx, method, requestPath, query, body)
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		respBody, err := c.rawRequest(ctx, method, requestPath, query, body)
		if err == nil {
			return respBody, nil
		}

		var apiErr *APIError
		if !errors.As(err, &apiErr) || !IsRateLimited(err) || attempt == c.maxRetries {
			return nil, err
		}
		lastErr = err

		wait := c.retryWait(attempt, apiErr.RetryAfter)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}

	// Unreachable: WithRetry rejects a negative maxRetries, so the loop
	// above always runs at least once and always returns.
	// Kept as a guard because the alternative failure mode is silent -
	// a nil body with a nil error reads to the caller as a successful
	// request that returned nothing.
	if lastErr == nil {
		return nil, fmt.Errorf("firezone: retry loop made no attempts with a budget of %d; this is a bug in the SDK", c.maxRetries)
	}
	return nil, lastErr
}

// retryWait returns how long to sleep before the next attempt.
//
// Retry-After is treated as a floor, never a target: waiting less than
// the server asked for guarantees another 429, so jitter is added on top
// of it and never subtracted from it.
//
// It is only a floor, though. The header says when one token frees up,
// not when this particular caller gets it - under concurrency the token
// is usually taken by someone else. Backing off exponentially on top of
// the floor is what lets a request that keeps losing eventually wait
// long enough to win, instead of retrying every second until its budget
// runs out.
func (c *Client) retryWait(attempt int, retryAfter time.Duration) time.Duration {
	wait := max(retryAfter, exponentialBackoff(attempt))
	wait = min(wait, c.maxRetryWait())
	return wait + retryJitter(wait)
}

func (c *Client) maxRetryWait() time.Duration {
	if c.retryMaxWait > 0 {
		return c.retryMaxWait
	}
	return defaultMaxRetryWait
}

// exponentialBackoff returns 1s, 2s, 4s, 8s, ... for attempt 0, 1, 2, 3.
// The caller caps it; this only guards the arithmetic.
func exponentialBackoff(attempt int) time.Duration {
	// Bail out before math.Pow overflows into +Inf, which would convert
	// to a nonsense Duration.
	if attempt >= 30 {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(math.Pow(2, float64(attempt))) * time.Second
}

// retryJitter returns a random padding of up to half the base wait,
// capped at maxRetryJitter, so concurrent callers rate limited at the
// same moment don't all retry at the same moment too.
func retryJitter(base time.Duration) time.Duration {
	window := min(base/2, maxRetryJitter)
	if window <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(window)))
}
