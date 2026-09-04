package firezone

import "time"

// ExportedRetryWait exposes retryWait to the external firezone_test
// package. Backoff timing is worth testing directly - driving it through
// an httptest server would mean actually sleeping out the escalation.
func ExportedRetryWait(c *Client, attempt int, retryAfter time.Duration) time.Duration {
	return c.retryWait(attempt, retryAfter)
}
