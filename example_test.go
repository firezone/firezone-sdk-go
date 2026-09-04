package firezone_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	firezone "github.com/firezone/firezone-sdk-go"
)

func ExampleNewClient() {
	client, err := firezone.NewClient("https://api.firezone.dev", os.Getenv("FIREZONE_TOKEN"))
	if err != nil {
		// Only a malformed base URL gets here - it must carry an
		// http/https scheme and a host, and no query or fragment.
		log.Fatal(err)
	}

	site, err := client.Sites.Create(context.Background(), &firezone.CreateSiteRequest{
		Name: "primary-dc",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(site.Name)
}

// The API reports a duplicate name as a validation error rather than a
// conflict, with the offending field named in ValidationErrors.
func ExampleIsValidation() {
	var client *firezone.Client // see [NewClient]

	_, err := client.Sites.Create(context.Background(), &firezone.CreateSiteRequest{Name: "primary-dc"})
	if firezone.IsValidation(err) {
		var apiErr *firezone.APIError
		if errors.As(err, &apiErr) {
			for field, messages := range apiErr.ValidationErrors {
				fmt.Printf("%s: %v\n", field, messages)
			}
		}
	}
}

// Paging is an explicit cursor loop: keep requesting until the metadata
// stops handing back a next-page cursor.
func ExampleListOptions() {
	var client *firezone.Client // see [NewClient]

	opts := &firezone.ResourceListOptions{
		ListOptions: firezone.ListOptions{Limit: 100},
	}
	for {
		page, err := client.Resources.List(context.Background(), opts)
		if err != nil {
			log.Fatal(err)
		}
		for _, resource := range page.Data {
			fmt.Println(resource.Name)
		}
		if page.Metadata.NextPage == "" {
			break
		}
		opts.PageCursor = page.Metadata.NextPage
	}
}

// Update requests are merge-patch: a field left nil keeps its current
// value, so this renames a Resource without disturbing anything else.
func ExampleResourcesService_Update() {
	var client *firezone.Client // see [NewClient]

	updated, err := client.Resources.Update(context.Background(), "resource-id",
		&firezone.UpdateResourceRequest{Name: "postgres-prod"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(updated.Name)
}

// Clear removes a nullable field, which no plain string value can
// express: a nil pointer means "leave it alone", so there would
// otherwise be no way to say "set this to nothing".
func ExampleClear() {
	var client *firezone.Client // see [NewClient]

	_, err := client.Resources.Update(context.Background(), "resource-id",
		&firezone.UpdateResourceRequest{
			// Remove the description; leave every other field untouched.
			AddressDescription: firezone.Clear[string](),
			// Move the Resource to another Site.
			SiteID: firezone.Set("site-id"),
		})
	if err != nil {
		log.Fatal(err)
	}
}

// The embedded lists are pointers for the same reason: a nil pointer
// leaves them alone, and a pointer to an empty slice removes them all.
func ExampleUpdatePolicyRequest_conditions() {
	var client *firezone.Client // see [NewClient]

	// Restrict the Policy to two countries.
	_, err := client.Policies.Update(context.Background(), "policy-id",
		&firezone.UpdatePolicyRequest{
			Conditions: &[]firezone.Condition{{
				Property: firezone.ConditionPropertyRemoteIPLocationRegion,
				Operator: firezone.ConditionOperatorIsIn,
				Values:   []string{"US", "CA"},
			}},
		})
	if err != nil {
		log.Fatal(err)
	}

	// Remove every condition, granting access unconditionally.
	if _, err := client.Policies.Update(context.Background(), "policy-id",
		&firezone.UpdatePolicyRequest{Conditions: &[]firezone.Condition{}}); err != nil {
		log.Fatal(err)
	}
}

// Gateways are nested under their Site, matching the API's own URLs.
// The token comes back exactly once, on provisioning.
func ExampleSitesService_Gateways() {
	var client *firezone.Client // see [NewClient]

	gateway, err := client.Sites.Gateways("site-id").Provision(context.Background(),
		&firezone.ProvisionGatewayRequest{Name: "gw-nyc-1"})
	if err != nil {
		log.Fatal(err)
	}

	// Store this now - the API never exposes it again.
	fmt.Println(gateway.Token)
}

// Retries are on by default and cover HTTP 429 only, honouring the
// API's Retry-After header. Tune the budget when a large concurrent run
// exhausts it.
func ExampleWithRetry() {
	client, err := firezone.NewClient("https://api.firezone.dev", os.Getenv("FIREZONE_TOKEN"),
		firezone.WithRetry(true, 20),
	)
	if err != nil {
		log.Fatal(err)
	}
	_ = client
}
