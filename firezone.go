// Package firezone provides a hand-written Go client for the Firezone
// REST API (https://www.firezone.dev).
//
// Construct a [Client] with [NewClient], then call methods on its
// resource services (Sites, Resources, Policies, Groups, Actors, and
// Gateways nested under Sites):
//
//	client, err := firezone.NewClient("https://api.firezone.dev", token)
//	site, err := client.Sites.Create(ctx, &firezone.CreateSiteRequest{Name: "primary-dc"})
//
// The API is currently unversioned - baseURL is the bare API host, with
// no path prefix of any kind. (URL path versioning was tried and rolled
// back before shipping; if it returns, it'll live in exactly one place
// here rather than every call site.)
//
// A [Client] is safe for concurrent use by multiple goroutines.
//
// Update requests are merge-patch: a field left at its zero value is
// omitted and keeps its current value on the server. Fields the API
// allows to be null are typed [Null] so they can be cleared as well as
// set - see [Clear] and [Set].
package firezone

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"runtime"
	"strconv"
	"time"
)

// requestBody is a request body captured as raw bytes rather than an
// io.Reader, so requestWithRetry can construct a fresh reader for each
// retry attempt - an io.Reader can only be drained once, which would
// silently send an empty body on any attempt after the first.
type requestBody []byte

func (b requestBody) reader() io.Reader {
	if b == nil {
		return nil
	}
	return bytes.NewReader(b)
}

// Version is this SDK's released version, following semantic
// versioning. It is sent as part of the default User-Agent, so a
// Firezone operator can tell which client version a request came from.
//
// It is a constant rather than something read from build info, because
// build info reports "(devel)" whenever the module is built rather than
// consumed. Bump it as part of cutting a release - see CONTRIBUTING.md.
const Version = "0.1.0"

// defaultUserAgent identifies this SDK and its version, plus the Go
// runtime it was built with - the latter is worth having when a
// server-side problem turns out to be specific to one Go release's
// TLS or HTTP behavior. It carries no OS or architecture, which would
// narrow a request to a machine without helping diagnose anything.
//
// Override it wholesale with [WithUserAgent]; a caller that wants to
// identify itself while keeping this information can build a string
// from [Version].
var defaultUserAgent = fmt.Sprintf("firezone-sdk-go/%s (%s)", Version, runtime.Version())

// String returns a pointer to s. Useful for optional string fields
// (e.g. [GroupListOptions.DirectoryID]) where a plain string's zero
// value can't distinguish "not set" from "set to the empty string".
func String(s string) *string {
	return &s
}

// Client is a Firezone REST API client.
//
// A Client is safe for concurrent use by multiple goroutines: it holds
// no mutable state once [NewClient] returns, and each request builds its
// own URL and body.
type Client struct {
	baseURL    *url.URL
	token      string
	httpClient *http.Client
	userAgent  string

	requestTimeout time.Duration

	retryEnabled bool
	maxRetries   int
	retryMaxWait time.Duration

	// Sites manages Sites and, nested under them, Gateways.
	Sites *SitesService
	// Resources manages Resources.
	Resources *ResourcesService
	// Policies manages Policies.
	Policies *PoliciesService
	// Groups manages Groups and, nested under them, memberships.
	Groups *GroupsService
	// Actors manages Actors.
	Actors *ActorsService
	// ClientDevices manages Client devices. Named for [ClientDevice],
	// since Clients is too easily confused with this type itself.
	ClientDevices *ClientsService
	// EmailOTPAuthProviders reads Email OTP auth providers (read-only).
	EmailOTPAuthProviders *EmailOTPAuthProvidersService
	// OIDCAuthProviders reads generic OIDC auth providers (read-only).
	OIDCAuthProviders *OIDCAuthProvidersService
	// GoogleAuthProviders reads Google Workspace auth providers
	// (read-only).
	GoogleAuthProviders *GoogleAuthProvidersService
	// EntraAuthProviders reads Microsoft Entra auth providers
	// (read-only).
	EntraAuthProviders *EntraAuthProvidersService
	// OktaAuthProviders reads Okta auth providers (read-only).
	OktaAuthProviders *OktaAuthProvidersService
	// EntraDirectories reads Microsoft Entra directory connections
	// (read-only).
	EntraDirectories *EntraDirectoriesService
	// GoogleDirectories reads Google Workspace directory connections
	// (read-only).
	GoogleDirectories *GoogleDirectoriesService
	// OktaDirectories reads Okta directory connections (read-only).
	OktaDirectories *OktaDirectoriesService
}

// Option configures a [Client].
//
// An Option returns an error rather than silently accepting bad input,
// so a misconfiguration is reported by [NewClient] at construction
// instead of surfacing later as a failed - or worse, a silently
// skipped - request. [NewClient] applies options in order and stops at
// the first error.
type Option func(*Client) error

// WithHTTPClient sets the underlying *http.Client used for requests,
// replacing the default described in [NewClient].
//
// Supplying a client is how a caller sets its own timeout, transport,
// or proxy configuration. Passing nil is an error: it would otherwise
// panic with a nil dereference on the first request, pointing at the
// request rather than at the option that caused it.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) error {
		if hc == nil {
			return errors.New("firezone: WithHTTPClient was given a nil *http.Client")
		}
		c.httpClient = hc
		return nil
	}
}

// WithUserAgent replaces the User-Agent header sent with every request.
// It replaces rather than extends the default, so a caller that wants to
// keep the SDK's identity should include it:
//
//	firezone.WithUserAgent("terraform-provider-firezone/2.1.0 firezone-sdk-go/" + firezone.Version)
func WithUserAgent(ua string) Option {
	return func(c *Client) error {
		if ua == "" {
			return errors.New("firezone: WithUserAgent was given an empty string; omit the option to keep the default")
		}
		c.userAgent = ua
		return nil
	}
}

// defaultRequestTimeout bounds a single HTTP attempt, from dialing to
// reading the response body. Change it with [WithRequestTimeout].
//
// A default is needed because neither http.DefaultClient nor a bare
// http.Client has a timeout: a caller who passes a context without a
// deadline would otherwise wait forever on an unresponsive endpoint,
// with the retry logic unable to help - nothing ever returns to be
// retried.
//
// The value is generous for a control-plane API where every response is
// a small JSON document.
const defaultRequestTimeout = 30 * time.Second

// newDefaultHTTPClient returns the *http.Client a [Client] uses unless
// [WithHTTPClient] says otherwise.
//
// It deliberately does not return http.DefaultClient. That value is
// process-global: a caller who wanted to adjust the SDK's transport
// would be adjusting it for every other package in the binary too, and
// any other package that mutates it would be adjusting the SDK's.
//
// It carries no Timeout of its own. The request timeout is applied to
// the context instead - see [WithRequestTimeout] - so that it composes
// with a caller-supplied client rather than being lost the moment one
// is passed.
func newDefaultHTTPClient() *http.Client {
	return &http.Client{}
}

// WithRequestTimeout bounds a single HTTP attempt, from dialing to
// reading the response body. The default is defaultRequestTimeout.
//
// This bounds one attempt, not a whole call: retry waits sit between
// requests rather than inside one, so a rate-limited call can still
// take longer overall, up to whatever budget [WithRetry] allows.
//
// It is applied to the request context rather than to the underlying
// http.Client, so nothing here overrides anything else. A Timeout on a
// client passed to [WithHTTPClient], a deadline already on the caller's
// context, and this option all apply together, and whichever expires
// first ends the attempt.
//
// Zero means the SDK imposes no timeout of its own, leaving the
// deadline entirely to the caller's context and http.Client. A negative
// duration is an error - context.WithTimeout would treat it as an
// already-expired deadline, failing every request before it is sent.
func WithRequestTimeout(d time.Duration) Option {
	return func(c *Client) error {
		if d < 0 {
			return fmt.Errorf("firezone: WithRequestTimeout was given a negative duration (%s)", d)
		}
		c.requestTimeout = d
		return nil
	}
}

// defaultMaxRetries is the retry budget a client gets unless
// [WithRetry] says otherwise.
//
// The API rate limits per account with a token bucket: 20 requests of
// burst, refilling at roughly one per second. A Terraform apply or
// destroy at the default parallelism of 10 drains that burst almost
// immediately and then proceeds at the refill rate, so a request can
// legitimately need to wait out a long queue ahead of it. With waits
// escalating to defaultMaxRetryWait, this budget covers a bit over two
// minutes of sustained throttling.
const defaultMaxRetries = 10

// WithRetry configures automatic retry-with-backoff on HTTP 429
// (rate limited) responses. Retries are enabled by default with a
// budget of defaultMaxRetries.
//
// Waits escalate exponentially, never drop below the response's
// Retry-After header, and always carry jitter so concurrent callers
// don't retry in lockstep.
//
// maxRetries is the number of retries after the first attempt, so zero
// means "try once, do not retry". A negative budget is rejected rather
// than clamped: the retry loop runs maxRetries+1 times, so a negative
// value would make it run zero times and return no response and no
// error at all. Nothing a caller can mean by it is worth guessing at.
func WithRetry(enabled bool, maxRetries int) Option {
	return func(c *Client) error {
		if maxRetries < 0 {
			return fmt.Errorf("firezone: WithRetry was given a negative retry budget (%d)", maxRetries)
		}
		c.retryEnabled = enabled
		c.maxRetries = maxRetries
		return nil
	}
}

// WithRetryMaxWait caps how long any single retry waits, bounding the
// exponential escalation. Zero or negative restores
// defaultMaxRetryWait.
//
// Raise it when a large Terraform run is still exhausting its budget:
// a higher cap buys more total patience per retry than more attempts
// at a low cap does.
func WithRetryMaxWait(d time.Duration) Option {
	return func(c *Client) error {
		c.retryMaxWait = d
		return nil
	}
}

// checkBaseURL rejects a base URL that url.Parse accepts but that can
// never produce a working request.
//
// url.Parse is permissive: it reads "api.firezone.dev" as a relative
// path with no scheme and no host, and returns no error. Left alone,
// that surfaces much later as "unsupported protocol scheme" from the
// first API call, pointing at the request rather than at the typo in
// the configuration that caused it.
func checkBaseURL(raw string, u *url.URL) error {
	switch {
	case u.Scheme != "http" && u.Scheme != "https":
		return fmt.Errorf("firezone: invalid base URL %q: scheme must be http or https, got %q", raw, u.Scheme)
	case u.Host == "":
		return fmt.Errorf("firezone: invalid base URL %q: missing host", raw)
	// A query or fragment on the base URL is silently dropped on any
	// request that sets its own query, so it would apply to some calls
	// and not others. Reject it rather than half-honor it.
	case u.RawQuery != "" || u.ForceQuery:
		return fmt.Errorf("firezone: invalid base URL %q: must not include a query string", raw)
	case u.Fragment != "":
		return fmt.Errorf("firezone: invalid base URL %q: must not include a fragment", raw)
	}
	return nil
}

// NewClient constructs a Firezone API client. baseURL is the bare API
// host (e.g. "https://api.firezone.dev") - do not include a version
// segment. token is the Bearer token for an api_client actor.
//
// Requests go through an *http.Client private to this SDK, and each
// attempt is bounded by defaultRequestTimeout. Pass [WithHTTPClient] to
// supply your own client and [WithRequestTimeout] to change the bound.
func NewClient(baseURL, token string, opts ...Option) (*Client, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("firezone: invalid base URL %q: %w", baseURL, err)
	}
	if err := checkBaseURL(baseURL, parsed); err != nil {
		return nil, err
	}

	c := &Client{
		baseURL:        parsed,
		token:          token,
		httpClient:     newDefaultHTTPClient(),
		requestTimeout: defaultRequestTimeout,
		userAgent:      defaultUserAgent,
		retryEnabled:   true,
		maxRetries:     defaultMaxRetries,
	}

	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}

	c.Sites = &SitesService{client: c}
	c.Resources = &ResourcesService{client: c}
	c.Policies = &PoliciesService{client: c}
	c.Groups = &GroupsService{client: c}
	c.Actors = &ActorsService{client: c}
	c.ClientDevices = &ClientsService{client: c}
	c.EmailOTPAuthProviders = &EmailOTPAuthProvidersService{client: c}
	c.OIDCAuthProviders = &OIDCAuthProvidersService{client: c}
	c.GoogleAuthProviders = &GoogleAuthProvidersService{client: c}
	c.EntraAuthProviders = &EntraAuthProvidersService{client: c}
	c.OktaAuthProviders = &OktaAuthProvidersService{client: c}
	c.EntraDirectories = &EntraDirectoriesService{client: c}
	c.GoogleDirectories = &GoogleDirectoriesService{client: c}
	c.OktaDirectories = &OktaDirectoriesService{client: c}

	return c, nil
}

// ErrNilRequest is returned when a Create or Update method is called
// with a nil request body. Test for it with [errors.Is]:
//
//	if errors.Is(err, firezone.ErrNilRequest) { ... }
//
// It is returned before any request is made. Without it a nil request
// encodes as {"site": null} rather than being caught: v is an any
// holding a typed nil pointer, so a plain v == nil check is false and
// json.Marshal happily writes the null.
var ErrNilRequest = errors.New("request must not be nil")

// wrapBody marshals v as JSON, nested under key, matching the API's
// request body shape (e.g. {"site": {"name": "..."}}).
func wrapBody(key string, v any) (requestBody, error) {
	if isNilPointer(v) {
		return nil, fmt.Errorf("firezone: %s %w", key, ErrNilRequest)
	}
	body, err := json.Marshal(map[string]any{key: v})
	if err != nil {
		return nil, fmt.Errorf("firezone: encoding request body: %w", err)
	}
	return requestBody(body), nil
}

// isNilPointer reports whether v is a nil pointer, including one boxed
// in a non-nil any - the shape every Create and Update method produces
// when handed a nil request struct.
//
// Only pointers are checked. The call sites that wrap a slice build it
// with make, so it is never nil, and a nil slice would in any case mean
// "no members", which is a request worth sending rather than an error.
func isNilPointer(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	return rv.Kind() == reflect.Pointer && rv.IsNil()
}

// dataEnvelope mirrors the {"data": ...} shape every non-list API
// response is wrapped in.
type dataEnvelope[T any] struct {
	Data T `json:"data"`
}

// listEnvelope mirrors the {"data": [...], "metadata": {...}} shape
// every list API response is wrapped in.
type listEnvelope[T any] struct {
	Data     []T              `json:"data"`
	Metadata pageMetadataBody `json:"metadata"`
}

// rawRequest performs a single HTTP round trip against the API, with no
// retry logic - callers needing retry-on-429 behavior use requestWithRetry.
// The returned body is the raw response bytes; a non-2xx status yields a
// non-nil *APIError.
//
// requestPath must already be escaped, which is what [buildPath] returns.
// Every call site builds its path that way, so a caller-supplied ID can
// never introduce a path separator.
func (c *Client) rawRequest(ctx context.Context, method, requestPath string, query url.Values, body requestBody) ([]byte, error) {
	// The timeout covers reading the response body as well as the round
	// trip, which is why it is cancelled on the way out of this function
	// rather than handed back to the caller: rawRequest reads the body
	// to completion before returning, so nothing outstanding depends on
	// the context afterwards.
	if c.requestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.requestTimeout)
		defer cancel()
	}

	u, err := resolvePath(c.baseURL, requestPath)
	if err != nil {
		return nil, err
	}
	if query != nil {
		u.RawQuery = query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body.reader())
	if err != nil {
		return nil, fmt.Errorf("firezone: building request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("firezone: performing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("firezone: reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseAPIError(resp, respBody)
	}

	return respBody, nil
}

// do performs a request and, on success, unmarshals the response's
// "data" field into out. out may be nil for responses with no body to
// capture (e.g. DELETE).
func (c *Client) do(ctx context.Context, method, requestPath string, query url.Values, body requestBody, out any) error {
	respBody, err := c.requestWithRetry(ctx, method, requestPath, query, body)
	if err != nil {
		return err
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}

	var env dataEnvelope[json.RawMessage]
	if err := json.Unmarshal(respBody, &env); err != nil {
		return fmt.Errorf("firezone: decoding response: %w", err)
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("firezone: decoding response data: %w", err)
	}
	return nil
}

// doList performs a request and unmarshals the response's "data" and
// "metadata" fields into a Page[T]. It's a package-level generic
// function (not a Client method) because Go methods can't introduce
// their own type parameters.
func doList[T any](ctx context.Context, c *Client, method, requestPath string, query url.Values) (*Page[T], error) {
	respBody, err := c.requestWithRetry(ctx, method, requestPath, query, nil)
	if err != nil {
		return nil, err
	}

	var env listEnvelope[T]
	if err := json.Unmarshal(respBody, &env); err != nil {
		return nil, fmt.Errorf("firezone: decoding list response: %w", err)
	}

	return &Page[T]{Data: env.Data, Metadata: env.Metadata.toMetadata()}, nil
}

// listOptionsToQuery converts ListOptions into the API's query
// parameters (limit, page_cursor). Always returns a non-nil (possibly
// empty) url.Values, so callers can safely add further query
// parameters to the result.
func listOptionsToQuery(opts *ListOptions) url.Values {
	q := url.Values{}
	if opts == nil {
		return q
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.PageCursor != "" {
		q.Set("page_cursor", opts.PageCursor)
	}
	return q
}

// filterQuery builds the query for a List method: opts' pagination
// parameters plus zero or more "key=value" filters, each set only when
// its value is non-empty. Every List method with resource-specific
// filters funnels through this so filter-building stays consistent.
func filterQuery(opts ListOptions, filters ...[2]string) url.Values {
	q := listOptionsToQuery(&opts)
	for _, kv := range filters {
		if kv[1] != "" {
			q.Set(kv[0], kv[1])
		}
	}
	return q
}
