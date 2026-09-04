//go:build integration

// Acceptance tests that run against a real Firezone portal.
//
// Everything else in this package tests against httptest stubs, which
// can only prove the SDK is self-consistent: the fixtures are written by
// the same hand as the code they check, so a wrong assumption about the
// API is invisible. These tests are the only ones that can disagree with
// the SDK about how the server behaves.
//
// They need a portal and a token:
//
//	export FIREZONE_ENDPOINT=https://localhost:13001
//	export FIREZONE_TOKEN=$(...)                    # see the README
//	export FIREZONE_CA_CERT=.../priv/cert/selfsigned.pem
//	mise run test-acceptance
//
// With either variable unset every test skips, so an untagged
// `go test ./...` never touches the network. CI does not run these - it
// only vets them (`go vet -tags=integration`), which is what keeps them
// compiling as the SDK changes.
//
// Tests create real objects. Every one is named with a run-scoped prefix
// and removed by t.Cleanup, so a run only ever deletes what it made and
// debris from a crashed run is identifiable by prefix.
package firezone_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	firezone "github.com/firezone/firezone-sdk-go"
)

const (
	envEndpoint = "FIREZONE_ENDPOINT"
	envToken    = "FIREZONE_TOKEN"
	// envCACert points at a PEM certificate to trust in addition to the
	// system roots. The dev server serves the API over HTTPS with a
	// self-signed certificate, which Go rejects unless it has been added
	// to the machine's trust store - set this instead of relying on
	// whether it happens to have been.
	envCACert = "FIREZONE_CA_CERT"

	// namePrefix marks every object these tests create. Anything left
	// behind by a crashed run is findable with it.
	namePrefix = "gosdk"
)

// runID distinguishes objects from concurrent or repeated runs, so a
// name collision can't fail a test for reasons that have nothing to do
// with the SDK.
var runID = fmt.Sprintf("%d%04d", time.Now().Unix()%100000, rand.IntN(10000))

// nameSeq keeps names unique within a run.
var nameSeq atomic.Int64

// uniqueName returns a run-scoped, collision-free object name.
func uniqueName(kind string) string {
	return fmt.Sprintf("%s-%s-%s-%d", namePrefix, runID, kind, nameSeq.Add(1))
}

// integrationClient returns a client for the portal under test, skipping
// the test when the environment doesn't describe one.
func integrationClient(t *testing.T) *firezone.Client {
	t.Helper()

	endpoint, token := os.Getenv(envEndpoint), os.Getenv(envToken)
	if endpoint == "" || token == "" {
		t.Skipf("set %s and %s to run acceptance tests", envEndpoint, envToken)
	}

	opts := []firezone.Option{firezone.WithUserAgent("firezone-sdk-go-acceptance-tests")}
	if caPath := os.Getenv(envCACert); caPath != "" {
		opts = append(opts, firezone.WithHTTPClient(httpClientTrusting(t, caPath)))
	}

	client, err := firezone.NewClient(endpoint, token, opts...)
	if err != nil {
		t.Fatalf("building client for %s: %v", endpoint, err)
	}
	return client
}

// httpClientTrusting returns a client that trusts the PEM certificate at
// path on top of the system roots.
func httpClientTrusting(t *testing.T, path string) *http.Client {
	t.Helper()

	pem, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s=%s: %v", envCACert, path, err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pem) {
		t.Fatalf("%s=%s: no certificate found in the file", envCACert, path)
	}

	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		},
	}
}

func ctx() context.Context { return context.Background() }

// cleanup registers a deletion that tolerates the object already being
// gone - a test that deletes its own subject as part of what it asserts
// shouldn't fail in teardown for succeeding.
func cleanup(t *testing.T, what string, del func() error) {
	t.Helper()
	t.Cleanup(func() {
		if err := del(); err != nil && !firezone.IsNotFound(err) {
			t.Errorf("cleanup: deleting %s: %v", what, err)
		}
	})
}

// --- scratch fixtures -------------------------------------------------

func newSite(t *testing.T, c *firezone.Client) *firezone.Site {
	t.Helper()
	site, err := c.Sites.Create(ctx(), &firezone.CreateSiteRequest{Name: uniqueName("site")})
	if err != nil {
		t.Fatalf("creating scratch Site: %v", err)
	}
	cleanup(t, "Site "+site.ID, func() error { return c.Sites.Delete(ctx(), site.ID) })
	return site
}

func newGroup(t *testing.T, c *firezone.Client) *firezone.Group {
	t.Helper()
	group, err := c.Groups.Create(ctx(), &firezone.CreateGroupRequest{Name: uniqueName("group")})
	if err != nil {
		t.Fatalf("creating scratch Group: %v", err)
	}
	cleanup(t, "Group "+group.ID, func() error { return c.Groups.Delete(ctx(), group.ID) })
	return group
}

func newActor(t *testing.T, c *firezone.Client) *firezone.Actor {
	t.Helper()
	actor, err := c.Actors.Create(ctx(), &firezone.CreateActorRequest{
		Name: uniqueName("actor"),
		Type: firezone.ActorTypeServiceAccount,
	})
	if err != nil {
		t.Fatalf("creating scratch Actor: %v", err)
	}
	cleanup(t, "Actor "+actor.ID, func() error { return c.Actors.Delete(ctx(), actor.ID) })
	return actor
}

// newResource creates a CIDR Resource in its own Site. Each call uses a
// distinct address so concurrent runs don't collide on one.
func newResource(t *testing.T, c *firezone.Client, siteID string) *firezone.Resource {
	t.Helper()
	resource, err := c.Resources.Create(ctx(), &firezone.CreateResourceRequest{
		Name:               uniqueName("resource"),
		Type:               firezone.ResourceTypeCIDR,
		Address:            fmt.Sprintf("10.%d.%d.0/24", rand.IntN(256), rand.IntN(256)),
		AddressDescription: "created by the Go SDK acceptance tests",
		SiteID:             siteID,
	})
	if err != nil {
		t.Fatalf("creating scratch Resource: %v", err)
	}
	cleanup(t, "Resource "+resource.ID, func() error { return c.Resources.Delete(ctx(), resource.ID) })
	return resource
}

// --- 1. merge-patch semantics ----------------------------------------

// TestIntegration_MergePatchSemantics is the reason this file exists.
//
// The Null[T] design rests on a reading of the server's changeset code:
// that an absent key leaves a field alone, an explicit null clears it,
// and an empty string is replaced by the field's default. Every unit
// test asserting that only proves the SDK marshals what the SDK intends.
// This is the one place the server gets a vote - and it has already
// overruled one of those assumptions once.
func TestIntegration_MergePatchSemantics(t *testing.T) {
	c := integrationClient(t)
	site := newSite(t, c)

	t.Run("a nullable field survives an update that omits it", func(t *testing.T) {
		resource := newResource(t, c, site.ID)
		if resource.AddressDescription == "" {
			t.Fatal("fixture Resource has no address description to preserve")
		}

		updated, err := c.Resources.Update(ctx(), resource.ID, &firezone.UpdateResourceRequest{
			Name: uniqueName("resource"),
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if updated.AddressDescription != resource.AddressDescription {
			t.Errorf("AddressDescription = %q after an update that omitted it, want it unchanged (%q)",
				updated.AddressDescription, resource.AddressDescription)
		}
	})

	t.Run("Clear removes a nullable field", func(t *testing.T) {
		resource := newResource(t, c, site.ID)

		updated, err := c.Resources.Update(ctx(), resource.ID, &firezone.UpdateResourceRequest{
			AddressDescription: firezone.Clear[string](),
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if updated.AddressDescription != "" {
			t.Errorf("AddressDescription = %q after Clear, want it empty", updated.AddressDescription)
		}

		// Re-read rather than trusting the update response: the two can
		// disagree, and the stored value is what matters.
		got, err := c.Resources.Get(ctx(), resource.ID)
		if err != nil {
			t.Fatalf("Get after Clear: %v", err)
		}
		if got.AddressDescription != "" {
			t.Errorf("AddressDescription = %q on re-read after Clear, want it empty", got.AddressDescription)
		}
	})

	t.Run("Set replaces a nullable field", func(t *testing.T) {
		resource := newResource(t, c, site.ID)

		updated, err := c.Resources.Update(ctx(), resource.ID, &firezone.UpdateResourceRequest{
			AddressDescription: firezone.Set("replaced by the acceptance tests"),
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if want := "replaced by the acceptance tests"; updated.AddressDescription != want {
			t.Errorf("AddressDescription = %q, want %q", updated.AddressDescription, want)
		}
	})

	// The API's changeset replaces an empty string with the field's
	// default rather than storing it, and for a nullable field that
	// default is null - so Set("") clears, exactly as Clear does. The
	// SDK originally documented the opposite; this test is what
	// corrected it, and it is here to keep the documented behavior and
	// the real one from drifting apart again.
	t.Run("Set of the empty string clears the field, like Clear", func(t *testing.T) {
		resource := newResource(t, c, site.ID)

		updated, err := c.Resources.Update(ctx(), resource.ID, &firezone.UpdateResourceRequest{
			AddressDescription: firezone.Set(""),
		})
		if err != nil {
			t.Fatalf("Update with Set(\"\"): %v", err)
		}
		if updated.AddressDescription != "" {
			t.Errorf("AddressDescription = %q after Set(\"\"), want it cleared.\n"+
				"If the API now stores or rejects the empty string instead, the Null doc "+
				"comment and the README's \"Updating nullable fields\" section need updating.",
				updated.AddressDescription)
		}

		got, err := c.Resources.Get(ctx(), resource.ID)
		if err != nil {
			t.Fatalf("Get after Set(\"\"): %v", err)
		}
		if got.AddressDescription != "" {
			t.Errorf("AddressDescription = %q on re-read, want it cleared", got.AddressDescription)
		}
	})

	t.Run("an empty Filters slice removes every filter", func(t *testing.T) {
		resource := newResource(t, c, site.ID)

		withFilters, err := c.Resources.Update(ctx(), resource.ID, &firezone.UpdateResourceRequest{
			Filters: &[]firezone.Filter{{Protocol: firezone.FilterProtocolTCP, Ports: []string{"5432"}}},
		})
		if err != nil {
			t.Fatalf("Update adding a filter: %v", err)
		}
		if len(withFilters.Filters) != 1 {
			t.Fatalf("Filters = %+v after adding one, want exactly one", withFilters.Filters)
		}

		cleared, err := c.Resources.Update(ctx(), resource.ID, &firezone.UpdateResourceRequest{
			Filters: &[]firezone.Filter{},
		})
		if err != nil {
			t.Fatalf("Update clearing filters: %v", err)
		}
		if len(cleared.Filters) != 0 {
			t.Errorf("Filters = %+v after sending an empty slice, want none", cleared.Filters)
		}
	})

	t.Run("a nil Filters pointer leaves the filters alone", func(t *testing.T) {
		resource := newResource(t, c, site.ID)

		if _, err := c.Resources.Update(ctx(), resource.ID, &firezone.UpdateResourceRequest{
			Filters: &[]firezone.Filter{{Protocol: firezone.FilterProtocolTCP, Ports: []string{"443"}}},
		}); err != nil {
			t.Fatalf("Update adding a filter: %v", err)
		}

		updated, err := c.Resources.Update(ctx(), resource.ID, &firezone.UpdateResourceRequest{
			Name: uniqueName("resource"),
		})
		if err != nil {
			t.Fatalf("Update omitting filters: %v", err)
		}
		if len(updated.Filters) != 1 {
			t.Errorf("Filters = %+v after an update that omitted them, want the one filter preserved",
				updated.Filters)
		}
	})

	t.Run("an empty Conditions slice removes every Policy condition", func(t *testing.T) {
		group := newGroup(t, c)
		resource := newResource(t, c, site.ID)

		policy, err := c.Policies.Create(ctx(), &firezone.CreatePolicyRequest{
			GroupID:    group.ID,
			ResourceID: resource.ID,
			Conditions: []firezone.Condition{{
				Property: firezone.ConditionPropertyRemoteIPLocationRegion,
				Operator: firezone.ConditionOperatorIsIn,
				Values:   []string{"US"},
			}},
		})
		if err != nil {
			t.Fatalf("Create with a condition: %v", err)
		}
		cleanup(t, "Policy "+policy.ID, func() error { return c.Policies.Delete(ctx(), policy.ID) })

		if len(policy.Conditions) != 1 {
			t.Fatalf("Conditions = %+v on create, want exactly one", policy.Conditions)
		}

		cleared, err := c.Policies.Update(ctx(), policy.ID, &firezone.UpdatePolicyRequest{
			Conditions: &[]firezone.Condition{},
		})
		if err != nil {
			t.Fatalf("Update clearing conditions: %v", err)
		}
		if len(cleared.Conditions) != 0 {
			t.Errorf("Conditions = %+v after sending an empty slice, want none.\n"+
				"This is the case a plain []Condition could not express at all.", cleared.Conditions)
		}
	})

	t.Run("Clear removes a Policy description", func(t *testing.T) {
		group := newGroup(t, c)
		resource := newResource(t, c, site.ID)

		policy, err := c.Policies.Create(ctx(), &firezone.CreatePolicyRequest{
			GroupID:     group.ID,
			ResourceID:  resource.ID,
			Description: "created by the Go SDK acceptance tests",
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		cleanup(t, "Policy "+policy.ID, func() error { return c.Policies.Delete(ctx(), policy.ID) })

		cleared, err := c.Policies.Update(ctx(), policy.ID, &firezone.UpdatePolicyRequest{
			Description: firezone.Clear[string](),
		})
		if err != nil {
			t.Fatalf("Update clearing the description: %v", err)
		}
		if cleared.Description != "" {
			t.Errorf("Description = %q after Clear, want it empty", cleared.Description)
		}
	})
}

// --- 2. error envelopes ----------------------------------------------

// TestIntegration_ErrorEnvelopes checks that parseAPIError reads what the
// server actually sends. The unit tests assert against
// internal/testutil's hand-written problem+json, which is a guess at the
// real shape.
func TestIntegration_ErrorEnvelopes(t *testing.T) {
	c := integrationClient(t)

	// A syntactically valid UUID that will not exist.
	const missingID = "00000000-0000-4000-8000-000000000000"

	t.Run("404 on a missing Site", func(t *testing.T) {
		_, err := c.Sites.Get(ctx(), missingID)
		if !firezone.IsNotFound(err) {
			t.Fatalf("error = %v, want a 404", err)
		}

		var apiErr *firezone.APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("error %v is not an *APIError", err)
		}
		if apiErr.StatusCode != 404 {
			t.Errorf("StatusCode = %d, want 404", apiErr.StatusCode)
		}
		if apiErr.Title == "" {
			t.Error("Title is empty; the problem+json body did not parse as expected")
		}
	})

	t.Run("422 carries field-level validation errors", func(t *testing.T) {
		_, err := c.Sites.Create(ctx(), &firezone.CreateSiteRequest{Name: ""})
		if !firezone.IsValidation(err) {
			t.Fatalf("error = %v, want a 422", err)
		}

		var apiErr *firezone.APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("error %v is not an *APIError", err)
		}
		if len(apiErr.ValidationErrors) == 0 {
			t.Fatal("ValidationErrors is empty; the API's field-level detail is not being parsed")
		}
		if _, ok := apiErr.ValidationErrors["name"]; !ok {
			t.Errorf("ValidationErrors = %v, want an entry keyed \"name\"", apiErr.ValidationErrors)
		}
	})

	// A duplicate name is a 422 with a field-level error, not the 409 a
	// reader might expect: the server enforces uniqueness with a
	// changeset unique_constraint, which surfaces as validation. The
	// README's opening example said otherwise until this test ran.
	t.Run("a duplicate Site name is a validation error, not a conflict", func(t *testing.T) {
		name := uniqueName("dup")
		first, err := c.Sites.Create(ctx(), &firezone.CreateSiteRequest{Name: name})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		cleanup(t, "Site "+first.ID, func() error { return c.Sites.Delete(ctx(), first.ID) })

		_, err = c.Sites.Create(ctx(), &firezone.CreateSiteRequest{Name: name})
		if !firezone.IsValidation(err) {
			t.Fatalf("error = %v, want a 422; if the API has moved to 409 for duplicates, "+
				"the README's opening example and its Is* notes need updating", err)
		}
		if firezone.IsConflict(err) {
			t.Error("IsConflict(err) = true; a duplicate name should not be reported as a 409")
		}

		var apiErr *firezone.APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("error %v is not an *APIError", err)
		}
		if _, ok := apiErr.ValidationErrors["name"]; !ok {
			t.Errorf("ValidationErrors = %v, want an entry keyed \"name\"", apiErr.ValidationErrors)
		}
	})
}

// --- 3. pagination ---------------------------------------------------

// TestIntegration_Pagination walks a cursor to exhaustion. Cursor
// semantics - whether next_page is inclusive, whether count is the total
// or the page size - are pure assumption everywhere else.
func TestIntegration_Pagination(t *testing.T) {
	c := integrationClient(t)

	const created = 5
	want := map[string]bool{}
	for i := 0; i < created; i++ {
		want[newSite(t, c).ID] = false
	}

	seen := map[string]int{}
	var pages int
	opts := &firezone.SiteListOptions{ListOptions: firezone.ListOptions{Limit: 2}}
	for {
		page, err := c.Sites.List(ctx(), opts)
		if err != nil {
			t.Fatalf("List page %d: %v", pages+1, err)
		}
		pages++

		if len(page.Data) > 2 {
			t.Errorf("page %d returned %d Sites, want at most the requested limit of 2", pages, len(page.Data))
		}
		for _, site := range page.Data {
			seen[site.ID]++
			if _, ours := want[site.ID]; ours {
				want[site.ID] = true
			}
		}

		if page.Metadata.NextPage == "" {
			if page.Metadata.Count < created {
				t.Errorf("Metadata.Count = %d, want at least the %d Sites this test created - "+
					"count should be the total across pages, not the page size",
					page.Metadata.Count, created)
			}
			break
		}
		opts.PageCursor = page.Metadata.NextPage

		// A cursor that never terminates would otherwise hang the suite.
		if pages > 100 {
			t.Fatal("cursor did not terminate after 100 pages")
		}
	}

	for id, found := range want {
		if !found {
			t.Errorf("Site %s was created but never appeared while paging", id)
		}
	}
	for id, n := range seen {
		if n > 1 {
			t.Errorf("Site %s appeared on %d pages, want exactly one", id, n)
		}
	}
	t.Logf("walked %d pages at limit 2", pages)
}

// --- 4. gateway provisioning -----------------------------------------

// TestIntegration_GatewayProvisionAndRotate covers the one-time token
// secrets. Both are shown exactly once, so a fixture asserting their
// shape proves nothing about whether the API really returns them there.
func TestIntegration_GatewayProvisionAndRotate(t *testing.T) {
	c := integrationClient(t)
	site := newSite(t, c)
	gateways := c.Sites.Gateways(site.ID)

	provisioned, err := gateways.Provision(ctx(), &firezone.ProvisionGatewayRequest{
		Name: uniqueName("gw"),
	})
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	cleanup(t, "Gateway "+provisioned.ID, func() error { return gateways.Delete(ctx(), provisioned.ID) })

	if provisioned.ID == "" {
		t.Error("provisioned Gateway has no ID")
	}
	if provisioned.Token == "" {
		t.Error("Provision returned no Token; it is the only call that ever exposes one")
	}

	got, err := gateways.Get(ctx(), provisioned.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != provisioned.ID {
		t.Errorf("Get returned Gateway %s, want %s", got.ID, provisioned.ID)
	}

	page, err := gateways.List(ctx(), nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !containsGatewayID(page.Data, provisioned.ID) {
		t.Errorf("List did not include the newly provisioned Gateway %s", provisioned.ID)
	}

	rotated, err := gateways.RotateToken(ctx(), provisioned.ID)
	if err != nil {
		t.Fatalf("RotateToken: %v", err)
	}
	if rotated.Token == "" {
		t.Error("RotateToken returned no Token")
	}
	if rotated.Token == provisioned.Token {
		t.Error("RotateToken returned the original token; rotation must mint a new secret")
	}
	if rotated.ID == "" {
		t.Error("RotateToken returned no token ID")
	}
}

func containsGatewayID(gateways []firezone.Gateway, id string) bool {
	for _, g := range gateways {
		if g.ID == id {
			return true
		}
	}
	return false
}

// --- 5. CRUD round trips ---------------------------------------------

// TestIntegration_SiteCRUD and its siblings are shallow by design: they
// exist to prove each method's path, verb and envelope against a real
// router, which is what no stub can do.
func TestIntegration_SiteCRUD(t *testing.T) {
	c := integrationClient(t)

	name := uniqueName("site")
	site, err := c.Sites.Create(ctx(), &firezone.CreateSiteRequest{Name: name})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if site.Name != name {
		t.Errorf("Name = %q, want %q", site.Name, name)
	}

	got, err := c.Sites.Get(ctx(), site.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != site.ID {
		t.Errorf("Get returned %s, want %s", got.ID, site.ID)
	}

	renamed := uniqueName("site")
	updated, err := c.Sites.Update(ctx(), site.ID, &firezone.UpdateSiteRequest{Name: renamed})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != renamed {
		t.Errorf("Name = %q after Update, want %q", updated.Name, renamed)
	}

	page, err := c.Sites.List(ctx(), &firezone.SiteListOptions{Name: renamed})
	if err != nil {
		t.Fatalf("List filtered by name: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].ID != site.ID {
		t.Errorf("List by name returned %+v, want exactly the updated Site", page.Data)
	}

	if err := c.Sites.Delete(ctx(), site.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := c.Sites.Get(ctx(), site.ID); !firezone.IsNotFound(err) {
		t.Errorf("Get after Delete returned %v, want a 404", err)
	}
}

func TestIntegration_ResourceCRUD(t *testing.T) {
	c := integrationClient(t)
	site := newSite(t, c)

	name := uniqueName("resource")
	resource, err := c.Resources.Create(ctx(), &firezone.CreateResourceRequest{
		Name:    name,
		Type:    firezone.ResourceTypeCIDR,
		Address: fmt.Sprintf("10.%d.%d.0/24", rand.IntN(256), rand.IntN(256)),
		SiteID:  site.ID,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if resource.Type != firezone.ResourceTypeCIDR {
		t.Errorf("Type = %q, want cidr", resource.Type)
	}
	if resource.SiteID != site.ID {
		t.Errorf("SiteID = %q, want %q", resource.SiteID, site.ID)
	}

	if _, err := c.Resources.Get(ctx(), resource.ID); err != nil {
		t.Fatalf("Get: %v", err)
	}

	renamed := uniqueName("resource")
	updated, err := c.Resources.Update(ctx(), resource.ID, &firezone.UpdateResourceRequest{Name: renamed})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != renamed {
		t.Errorf("Name = %q after Update, want %q", updated.Name, renamed)
	}

	page, err := c.Resources.List(ctx(), &firezone.ResourceListOptions{SiteID: site.ID})
	if err != nil {
		t.Fatalf("List filtered by site_id: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].ID != resource.ID {
		t.Errorf("List by site_id returned %+v, want exactly the created Resource", page.Data)
	}

	if err := c.Resources.Delete(ctx(), resource.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := c.Resources.Get(ctx(), resource.ID); !firezone.IsNotFound(err) {
		t.Errorf("Get after Delete returned %v, want a 404", err)
	}
}

func TestIntegration_PolicyCRUD(t *testing.T) {
	c := integrationClient(t)
	site := newSite(t, c)
	group := newGroup(t, c)
	resource := newResource(t, c, site.ID)

	policy, err := c.Policies.Create(ctx(), &firezone.CreatePolicyRequest{
		GroupID:    group.ID,
		ResourceID: resource.ID,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if policy.GroupID != group.ID || policy.ResourceID != resource.ID {
		t.Errorf("Policy = %+v, want it to link Group %s to Resource %s", policy, group.ID, resource.ID)
	}

	if _, err := c.Policies.Get(ctx(), policy.ID); err != nil {
		t.Fatalf("Get: %v", err)
	}

	disabled, err := c.Policies.Disable(ctx(), policy.ID)
	if err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if !disabled.IsDisabled {
		t.Error("IsDisabled = false after Disable")
	}

	enabled, err := c.Policies.Enable(ctx(), policy.ID)
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if enabled.IsDisabled {
		t.Error("IsDisabled = true after Enable")
	}

	page, err := c.Policies.List(ctx(), &firezone.PolicyListOptions{GroupID: group.ID})
	if err != nil {
		t.Fatalf("List filtered by group_id: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].ID != policy.ID {
		t.Errorf("List by group_id returned %+v, want exactly the created Policy", page.Data)
	}

	if err := c.Policies.Delete(ctx(), policy.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := c.Policies.Get(ctx(), policy.ID); !firezone.IsNotFound(err) {
		t.Errorf("Get after Delete returned %v, want a 404", err)
	}
}

func TestIntegration_GroupCRUD(t *testing.T) {
	c := integrationClient(t)

	name := uniqueName("group")
	group, err := c.Groups.Create(ctx(), &firezone.CreateGroupRequest{Name: name})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if group.Name != name {
		t.Errorf("Name = %q, want %q", group.Name, name)
	}
	if group.IsSynced() {
		t.Error("IsSynced() = true for a Group created through the API")
	}

	if _, err := c.Groups.Get(ctx(), group.ID); err != nil {
		t.Fatalf("Get: %v", err)
	}

	renamed := uniqueName("group")
	updated, err := c.Groups.Update(ctx(), group.ID, &firezone.UpdateGroupRequest{Name: renamed})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != renamed {
		t.Errorf("Name = %q after Update, want %q", updated.Name, renamed)
	}

	// A pointer to the empty string filters to unsynced Groups, which is
	// the distinction GroupListOptions.DirectoryID exists to express.
	page, err := c.Groups.List(ctx(), &firezone.GroupListOptions{
		Name:        renamed,
		DirectoryID: firezone.String(""),
	})
	if err != nil {
		t.Fatalf("List filtered by name and unsynced: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].ID != group.ID {
		t.Errorf("List returned %+v, want exactly the updated Group", page.Data)
	}

	if err := c.Groups.Delete(ctx(), group.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := c.Groups.Get(ctx(), group.ID); !firezone.IsNotFound(err) {
		t.Errorf("Get after Delete returned %v, want a 404", err)
	}
}

func TestIntegration_ActorCRUD(t *testing.T) {
	c := integrationClient(t)

	name := uniqueName("actor")
	actor, err := c.Actors.Create(ctx(), &firezone.CreateActorRequest{
		Name: name,
		Type: firezone.ActorTypeServiceAccount,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if actor.Type != firezone.ActorTypeServiceAccount {
		t.Errorf("Type = %q, want service_account", actor.Type)
	}
	if actor.IsSynced() {
		t.Error("IsSynced() = true for an Actor created through the API")
	}

	if _, err := c.Actors.Get(ctx(), actor.ID); err != nil {
		t.Fatalf("Get: %v", err)
	}

	renamed := uniqueName("actor")
	updated, err := c.Actors.Update(ctx(), actor.ID, &firezone.UpdateActorRequest{Name: renamed})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != renamed {
		t.Errorf("Name = %q after Update, want %q", updated.Name, renamed)
	}

	disabled, err := c.Actors.Disable(ctx(), actor.ID)
	if err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if !disabled.IsDisabled {
		t.Error("IsDisabled = false after Disable")
	}

	enabled, err := c.Actors.Enable(ctx(), actor.ID)
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if enabled.IsDisabled {
		t.Error("IsDisabled = true after Enable")
	}

	page, err := c.Actors.List(ctx(), &firezone.ActorListOptions{Name: renamed})
	if err != nil {
		t.Fatalf("List filtered by name: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].ID != actor.ID {
		t.Errorf("List by name returned %+v, want exactly the updated Actor", page.Data)
	}

	if err := c.Actors.Delete(ctx(), actor.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := c.Actors.Get(ctx(), actor.ID); !firezone.IsNotFound(err) {
		t.Errorf("Get after Delete returned %v, want a 404", err)
	}
}

func TestIntegration_Memberships(t *testing.T) {
	c := integrationClient(t)
	group := newGroup(t, c)
	first, second := newActor(t, c), newActor(t, c)
	memberships := c.Groups.Memberships(group.ID)

	ids, err := memberships.ReplaceAll(ctx(), []string{first.ID})
	if err != nil {
		t.Fatalf("ReplaceAll: %v", err)
	}
	if len(ids) != 1 || ids[0] != first.ID {
		t.Errorf("ReplaceAll returned %v, want [%s]", ids, first.ID)
	}

	// Patch is the additive form: adding one and removing the other in a
	// single call is what distinguishes it from ReplaceAll.
	ids, err = memberships.Patch(ctx(), []string{second.ID}, []string{first.ID})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if len(ids) != 1 || ids[0] != second.ID {
		t.Errorf("Patch returned %v, want [%s]", ids, second.ID)
	}

	page, err := memberships.List(ctx(), nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].ID != second.ID {
		t.Errorf("List returned %+v, want exactly Actor %s", page.Data, second.ID)
	}

	ids, err = memberships.ReplaceAll(ctx(), []string{})
	if err != nil {
		t.Fatalf("ReplaceAll with an empty list: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("ReplaceAll([]) returned %v, want no members", ids)
	}
}

// TestIntegration_PoolMembers needs a static_device_pool Resource, which
// the API refuses to create - pools are made in the admin portal. The
// test discovers one and skips when the account has none.
func TestIntegration_PoolMembers(t *testing.T) {
	c := integrationClient(t)

	pools, err := c.Resources.List(ctx(), &firezone.ResourceListOptions{
		Type: firezone.ResourceTypeStaticDevicePool,
	})
	if err != nil {
		t.Fatalf("listing static device pools: %v", err)
	}
	if len(pools.Data) == 0 {
		t.Skip("no static_device_pool Resource in this account; create one in the admin portal to cover this")
	}

	pool := pools.Data[0]
	page, err := c.Resources.PoolMembers(pool.ID).List(ctx(), nil)
	if err != nil {
		t.Fatalf("PoolMembers.List for pool %s: %v", pool.ID, err)
	}
	t.Logf("pool %s has %d member(s) of %d total", pool.ID, len(page.Data), page.Metadata.Count)

	// A pool with members is the only chance to check that PoolMember
	// decodes; the spec marks id and name required and non-nullable.
	for i := range page.Data {
		member := page.Data[i]
		nonEmpty(t, "PoolMember.ID", member.ID)
		nonEmpty(t, "PoolMember.Name", member.Name)
		// LastSeenAt is nullable - a pooled device that has never
		// connected has none.
		t.Logf("member %s (%s) last seen %v", member.ID, member.Name, member.LastSeenAt)
	}

	// Membership is not modified here: the members are real enrolled
	// devices belonging to whoever owns this account, and ReplaceAll
	// would evict them. Deepen this only against a throwaway account.
}

// TestIntegration_ClientDevices is read-only: Client devices enroll
// themselves when a real client connects, so there is nothing for a test
// to create.
func TestIntegration_ClientDevices(t *testing.T) {
	c := integrationClient(t)

	page, err := c.ClientDevices.List(ctx(), nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(page.Data) == 0 {
		t.Skip("no enrolled Client devices in this account; connect a client to cover Get and Verify")
	}

	first := page.Data[0]
	got, err := c.ClientDevices.Get(ctx(), first.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != first.ID {
		t.Errorf("Get returned %s, want %s", got.ID, first.ID)
	}
	if got.FirezoneID == "" {
		t.Error("FirezoneID is empty; every enrolled device should carry one")
	}

	// Verify/Unverify and Update are left alone: they mutate devices
	// belonging to whoever owns this account.
}

// --- 6. read-only services -------------------------------------------

// checkReadOnly runs the shared contract for a read-only list/get
// service: every record decodes with its always-populated fields
// populated, Get agrees with List field for field, and the name filter
// is honoured by the server.
//
// It skips when the account has no records of this kind. That is a real
// gap rather than a pass - the field mappings for these types are only
// verified when something exists to verify them against.
func checkReadOnly[T any](
	t *testing.T,
	kind string,
	list func(nameFilter string) (*firezone.Page[T], error),
	get func(id string) (*T, error),
	idOf func(*T) string,
	nameOf func(*T) string,
	verify func(t *testing.T, rec *T),
) {
	t.Helper()

	page, err := list("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(page.Data) == 0 {
		t.Skipf("no %s configured in this account; its field mappings stay unverified", kind)
	}

	for i := range page.Data {
		rec := page.Data[i]
		t.Run(fmt.Sprintf("record %s", idOf(&rec)), func(t *testing.T) {
			verify(t, &rec)

			// Get and List render through the same view on the server,
			// so any difference is a bug in one of them - a field the
			// index omits, or a type that decodes differently.
			got, err := get(idOf(&rec))
			if err != nil {
				t.Fatalf("Get: %v", err)
			}
			if fromList, fromGet := mustJSON(t, rec), mustJSON(t, *got); fromList != fromGet {
				t.Errorf("List and Get disagree for %s:\n  List: %s\n   Get: %s",
					idOf(&rec), fromList, fromGet)
			}
		})
	}

	// The name filter is built by the SDK and honoured by the server;
	// unit tests only cover the half the SDK owns.
	first := page.Data[0]
	if name := nameOf(&first); name != "" {
		filtered, err := list(name)
		if err != nil {
			t.Fatalf("List filtered by name %q: %v", name, err)
		}
		var found bool
		for i := range filtered.Data {
			if idOf(&filtered.Data[i]) == idOf(&first) {
				found = true
			}
		}
		if !found {
			t.Errorf("List(name=%q) did not return %s; the server may not honour the filter",
				name, idOf(&first))
		}
	}
}

// mustJSON renders a value for comparison and for failure messages.
// Comparing marshalled JSON rather than using reflect.DeepEqual keeps
// time.Time values from differing over location or monotonic readings,
// and makes a mismatch readable.
func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshalling %T: %v", v, err)
	}
	return string(b)
}

func nonEmpty(t *testing.T, field, value string) {
	t.Helper()
	if value == "" {
		t.Errorf("%s is empty; the spec marks it required and non-nullable, so either the "+
			"JSON tag is wrong or this record is misconfigured", field)
	}
}

func nonZeroTime(t *testing.T, field string, value time.Time) {
	t.Helper()
	if value.IsZero() {
		t.Errorf("%s is the zero time; the spec marks it required and non-nullable, so either "+
			"the JSON tag is wrong or the timestamp did not parse", field)
	}
}

// verifyAuthProviderBase checks the fields every provider type shares.
//
// The session lifetimes are deliberately not asserted to be positive.
// They are nil whenever the provider sets no override, which is the
// normal state - asserting otherwise is what failed the first time these
// tests ran against a real portal, and it was the assertion that was
// wrong, not the data. A non-nil value must still be positive, since the
// server validates a configured lifetime against a minimum.
func verifyAuthProviderBase(t *testing.T, p *firezone.AuthProvider) {
	t.Helper()

	nonEmpty(t, "ID", p.ID)
	nonEmpty(t, "AccountID", p.AccountID)
	nonEmpty(t, "Name", p.Name)
	nonEmpty(t, "Issuer", p.Issuer)
	nonEmpty(t, "Context", p.Context)
	nonZeroTime(t, "InsertedAt", p.InsertedAt)
	nonZeroTime(t, "UpdatedAt", p.UpdatedAt)

	checkLifetime(t, "ClientSessionLifetimeSecs", p.ClientSessionLifetimeSecs)
	checkLifetime(t, "PortalSessionLifetimeSecs", p.PortalSessionLifetimeSecs)
}

func checkLifetime(t *testing.T, field string, secs *int) {
	t.Helper()
	if secs == nil {
		t.Logf("%s is nil (no override; Firezone's default applies)", field)
		return
	}
	if *secs <= 0 {
		t.Errorf("%s = %d, want a positive duration when it is set at all", field, *secs)
	}
}

func TestIntegration_AuthProviders(t *testing.T) {
	c := integrationClient(t)
	opts := func(name string) *firezone.AuthProviderListOptions {
		return &firezone.AuthProviderListOptions{Name: name}
	}

	t.Run("email OTP", func(t *testing.T) {
		checkReadOnly(t, "email OTP auth providers",
			func(n string) (*firezone.Page[firezone.EmailOTPAuthProvider], error) {
				return c.EmailOTPAuthProviders.List(ctx(), opts(n))
			},
			func(id string) (*firezone.EmailOTPAuthProvider, error) {
				return c.EmailOTPAuthProviders.Get(ctx(), id)
			},
			func(p *firezone.EmailOTPAuthProvider) string { return p.ID },
			func(p *firezone.EmailOTPAuthProvider) string { return p.Name },
			func(t *testing.T, p *firezone.EmailOTPAuthProvider) {
				verifyAuthProviderBase(t, &p.AuthProvider)
			})
	})

	t.Run("OIDC", func(t *testing.T) {
		checkReadOnly(t, "OIDC auth providers",
			func(n string) (*firezone.Page[firezone.OIDCAuthProvider], error) {
				return c.OIDCAuthProviders.List(ctx(), opts(n))
			},
			func(id string) (*firezone.OIDCAuthProvider, error) {
				return c.OIDCAuthProviders.Get(ctx(), id)
			},
			func(p *firezone.OIDCAuthProvider) string { return p.ID },
			func(p *firezone.OIDCAuthProvider) string { return p.Name },
			func(t *testing.T, p *firezone.OIDCAuthProvider) {
				verifyAuthProviderBase(t, &p.AuthProvider)
				nonEmpty(t, "ClientID", p.ClientID)
				nonEmpty(t, "DiscoveryDocumentURI", p.DiscoveryDocumentURI)
				nonEmpty(t, "EmailVerificationMethod", p.EmailVerificationMethod)
			})
	})

	t.Run("Google", func(t *testing.T) {
		checkReadOnly(t, "Google auth providers",
			func(n string) (*firezone.Page[firezone.GoogleAuthProvider], error) {
				return c.GoogleAuthProviders.List(ctx(), opts(n))
			},
			func(id string) (*firezone.GoogleAuthProvider, error) {
				return c.GoogleAuthProviders.Get(ctx(), id)
			},
			func(p *firezone.GoogleAuthProvider) string { return p.ID },
			func(p *firezone.GoogleAuthProvider) string { return p.Name },
			func(t *testing.T, p *firezone.GoogleAuthProvider) {
				verifyAuthProviderBase(t, &p.AuthProvider)
			})
	})

	t.Run("Entra", func(t *testing.T) {
		checkReadOnly(t, "Entra auth providers",
			func(n string) (*firezone.Page[firezone.EntraAuthProvider], error) {
				return c.EntraAuthProviders.List(ctx(), opts(n))
			},
			func(id string) (*firezone.EntraAuthProvider, error) {
				return c.EntraAuthProviders.Get(ctx(), id)
			},
			func(p *firezone.EntraAuthProvider) string { return p.ID },
			func(p *firezone.EntraAuthProvider) string { return p.Name },
			func(t *testing.T, p *firezone.EntraAuthProvider) {
				verifyAuthProviderBase(t, &p.AuthProvider)
				nonEmpty(t, "EmailClaim", p.EmailClaim)
			})
	})

	t.Run("Okta", func(t *testing.T) {
		checkReadOnly(t, "Okta auth providers",
			func(n string) (*firezone.Page[firezone.OktaAuthProvider], error) {
				return c.OktaAuthProviders.List(ctx(), opts(n))
			},
			func(id string) (*firezone.OktaAuthProvider, error) {
				return c.OktaAuthProviders.Get(ctx(), id)
			},
			func(p *firezone.OktaAuthProvider) string { return p.ID },
			func(p *firezone.OktaAuthProvider) string { return p.Name },
			func(t *testing.T, p *firezone.OktaAuthProvider) {
				verifyAuthProviderBase(t, &p.AuthProvider)
				nonEmpty(t, "ClientID", p.ClientID)
				nonEmpty(t, "OktaDomain", p.OktaDomain)
			})
	})
}

// logSyncState reports the nullable sync fields every directory type
// shares. They are only populated once a sync has run or failed, so they
// cannot be asserted - but seeing which are set is what tells us whether
// the mapping has ever been exercised against real data.
func logSyncState(t *testing.T, disabledReason, errorMessage string, syncedAt, erroredAt *time.Time) {
	t.Helper()
	t.Logf("nullable sync state: SyncedAt=%v ErroredAt=%v DisabledReason=%q ErrorMessage=%q",
		syncedAt, erroredAt, disabledReason, errorMessage)
}

func TestIntegration_Directories(t *testing.T) {
	c := integrationClient(t)
	opts := func(name string) *firezone.DirectoryListOptions {
		return &firezone.DirectoryListOptions{Name: name}
	}

	t.Run("Entra", func(t *testing.T) {
		checkReadOnly(t, "Entra directories",
			func(n string) (*firezone.Page[firezone.EntraDirectory], error) {
				return c.EntraDirectories.List(ctx(), opts(n))
			},
			func(id string) (*firezone.EntraDirectory, error) {
				return c.EntraDirectories.Get(ctx(), id)
			},
			func(d *firezone.EntraDirectory) string { return d.ID },
			func(d *firezone.EntraDirectory) string { return d.Name },
			func(t *testing.T, d *firezone.EntraDirectory) {
				nonEmpty(t, "ID", d.ID)
				nonEmpty(t, "AccountID", d.AccountID)
				nonEmpty(t, "Name", d.Name)
				nonEmpty(t, "TenantID", d.TenantID)
				nonEmpty(t, "EmailField", d.EmailField)
				nonZeroTime(t, "InsertedAt", d.InsertedAt)
				nonZeroTime(t, "UpdatedAt", d.UpdatedAt)
				logSyncState(t, d.DisabledReason, d.ErrorMessage, d.SyncedAt, d.ErroredAt)
			})
	})

	t.Run("Google", func(t *testing.T) {
		checkReadOnly(t, "Google directories",
			func(n string) (*firezone.Page[firezone.GoogleDirectory], error) {
				return c.GoogleDirectories.List(ctx(), opts(n))
			},
			func(id string) (*firezone.GoogleDirectory, error) {
				return c.GoogleDirectories.Get(ctx(), id)
			},
			func(d *firezone.GoogleDirectory) string { return d.ID },
			func(d *firezone.GoogleDirectory) string { return d.Name },
			func(t *testing.T, d *firezone.GoogleDirectory) {
				nonEmpty(t, "ID", d.ID)
				nonEmpty(t, "AccountID", d.AccountID)
				nonEmpty(t, "Name", d.Name)
				nonEmpty(t, "Domain", d.Domain)
				nonEmpty(t, "ImpersonationEmail", d.ImpersonationEmail)
				nonEmpty(t, "GroupSyncMode", d.GroupSyncMode)
				nonZeroTime(t, "InsertedAt", d.InsertedAt)
				nonZeroTime(t, "UpdatedAt", d.UpdatedAt)
				logSyncState(t, d.DisabledReason, d.ErrorMessage, d.SyncedAt, d.ErroredAt)
			})
	})

	t.Run("Okta", func(t *testing.T) {
		checkReadOnly(t, "Okta directories",
			func(n string) (*firezone.Page[firezone.OktaDirectory], error) {
				return c.OktaDirectories.List(ctx(), opts(n))
			},
			func(id string) (*firezone.OktaDirectory, error) {
				return c.OktaDirectories.Get(ctx(), id)
			},
			func(d *firezone.OktaDirectory) string { return d.ID },
			func(d *firezone.OktaDirectory) string { return d.Name },
			func(t *testing.T, d *firezone.OktaDirectory) {
				nonEmpty(t, "ID", d.ID)
				nonEmpty(t, "AccountID", d.AccountID)
				nonEmpty(t, "Name", d.Name)
				nonEmpty(t, "ClientID", d.ClientID)
				nonEmpty(t, "Kid", d.Kid)
				nonEmpty(t, "OktaDomain", d.OktaDomain)
				nonZeroTime(t, "InsertedAt", d.InsertedAt)
				nonZeroTime(t, "UpdatedAt", d.UpdatedAt)
				logSyncState(t, d.DisabledReason, d.ErrorMessage, d.SyncedAt, d.ErroredAt)
			})
	})
}

// leftovers reports objects a previous crashed run may have left behind,
// so they don't accumulate unnoticed in a long-lived dev account.
func TestIntegration_ReportLeftovers(t *testing.T) {
	c := integrationClient(t)

	page, err := c.Sites.List(ctx(), &firezone.SiteListOptions{
		ListOptions: firezone.ListOptions{Limit: 100},
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	var stale []string
	for _, site := range page.Data {
		if strings.HasPrefix(site.Name, namePrefix+"-") && !strings.Contains(site.Name, runID) {
			stale = append(stale, site.Name)
		}
	}
	if len(stale) > 0 {
		t.Logf("%d Site(s) left by earlier runs (delete by the %q prefix): %s",
			len(stale), namePrefix, strings.Join(stale, ", "))
	}
}
