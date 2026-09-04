package firezone

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// ErrMissingID is returned when a method is called with an empty or
// otherwise unusable resource ID. Test for it with [errors.Is]:
//
//	if errors.Is(err, firezone.ErrMissingID) { ... }
//
// It is returned before any request is made, so an ID that came from
// unpopulated config fails loudly rather than being sent as an empty
// path segment - which would silently address the collection endpoint
// instead (GET /sites rather than GET /sites/{id}).
var ErrMissingID = errors.New("must not be empty")

// checkID validates a caller-supplied path ID. name is the ID's role in
// the error message, e.g. "site ID".
//
// "." and ".." are rejected outright rather than escaped: url.PathEscape
// leaves them untouched (neither character is reserved), so they would
// reach the wire as-is and any path-normalizing proxy in front of the
// API would resolve them into a different path. Every other unsafe
// character, "/" included, is handled by escaping in buildPath.
func checkID(name, id string) error {
	if id == "" {
		return fmt.Errorf("firezone: %s %w", name, ErrMissingID)
	}
	if id == "." || id == ".." {
		return fmt.Errorf("firezone: %s %q is not a valid path segment", name, id)
	}
	return nil
}

// buildPath joins segments into a request path, percent-escaping each
// one so it stays a single path segment.
//
// Escaping is what keeps a caller-supplied ID from redirecting the
// request: without it an ID of "../actors/evil" turns
// GET /sites/{id} into GET /actors/evil, addressing a different
// resource entirely. Escaped, it stays one (nonexistent) segment and
// the API answers 404.
//
// The result is already escaped, so it goes into url.URL.RawPath rather
// than url.URL.Path - see resolvePath.
func buildPath(segments ...string) string {
	escaped := make([]string, len(segments))
	for i, s := range segments {
		escaped[i] = url.PathEscape(s)
	}
	return strings.Join(escaped, "/")
}

// resolvePath resolves an escaped request path against the base URL,
// returning the absolute URL to request.
//
// It deliberately does not use path.Join: that operates on the decoded
// path and cleans "." and ".." segments, which would undo buildPath's
// escaping work by resolving traversal that the escaping was there to
// contain. Instead the escaped form is assembled directly and stored in
// RawPath, with the decoded form in Path - the pairing url.URL uses to
// preserve escaping through String().
func resolvePath(base *url.URL, requestPath string) (*url.URL, error) {
	escaped := strings.TrimSuffix(base.EscapedPath(), "/") + "/" + requestPath

	decoded, err := url.PathUnescape(escaped)
	if err != nil {
		return nil, fmt.Errorf("firezone: building request path: %w", err)
	}

	u := *base
	u.Path = decoded
	u.RawPath = escaped
	return &u, nil
}
