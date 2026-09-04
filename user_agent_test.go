package firezone_test

import (
	"context"
	"net/http"
	"regexp"
	"runtime"
	"strings"
	"testing"

	firezone "github.com/firezone/firezone-sdk-go"
	"github.com/firezone/firezone-sdk-go/internal/testutil"
)

// newRecordingClient returns a client whose stub server records the
// User-Agent of whatever request reaches it.
func newRecordingClient(t *testing.T, ua *string, opts ...firezone.Option) *firezone.Client {
	t.Helper()
	return testutil.NewClientWithOptions(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*ua = r.Header.Get("User-Agent")
		testutil.JSONResponse(http.StatusOK, map[string]any{"data": map[string]any{"id": "site-1"}})(w, r)
	}), opts...)
}

// TestVersion pins the shape of Version. A version that isn't semver
// would break a consumer parsing it, and it ends up on the wire.
func TestVersion(t *testing.T) {
	if !regexp.MustCompile(`^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`).MatchString(firezone.Version) {
		t.Errorf("Version = %q, want a semantic version like 1.2.3", firezone.Version)
	}
}

func TestDefaultUserAgent(t *testing.T) {
	var ua string
	client := newRecordingClient(t, &ua)
	if _, err := client.Sites.Get(context.Background(), "site-1"); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}

	if !strings.HasPrefix(ua, "firezone-sdk-go/"+firezone.Version) {
		t.Errorf("User-Agent = %q, want it to start with firezone-sdk-go/%s", ua, firezone.Version)
	}
	if !strings.Contains(ua, runtime.Version()) {
		t.Errorf("User-Agent = %q, want it to name the Go runtime (%s)", ua, runtime.Version())
	}
}

func TestWithUserAgentReplacesTheDefault(t *testing.T) {
	const custom = "terraform-provider-firezone/2.1.0"

	var ua string
	client := newRecordingClient(t, &ua, firezone.WithUserAgent(custom))
	if _, err := client.Sites.Get(context.Background(), "site-1"); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}

	if ua != custom {
		t.Errorf("User-Agent = %q, want exactly %q - WithUserAgent replaces rather than extends", ua, custom)
	}
}
