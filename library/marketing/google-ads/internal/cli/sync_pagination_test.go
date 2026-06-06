// Copyright 2026 Cathryn Lavery and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

// TestNonPaginatedResourcesSuppressesParams guards the F0 sync fix: the
// listAccessibleCustomers endpoint (resource "customers") takes no pagination,
// cursor, or filter query parameters and rejects unknown params with HTTP 400
// ("Invalid JSON payload received. Unknown name \"limit\""). The sync loop must
// therefore send NO generated query params for that resource. This test pins
// the data that drives that suppression so a regen or refactor cannot silently
// re-introduce the limit= param that left the mirror unhydrated.
func TestNonPaginatedResourcesSuppressesParams(t *testing.T) {
	if !nonPaginatedResources["customers"] {
		t.Fatalf("customers must be in nonPaginatedResources: listAccessibleCustomers 400s on any query param (limit/after/since)")
	}
}

// TestDefaultSyncRootIsNonPaginated asserts that every resource sync would run
// by default is recognized as non-paginated, since "customers" is both the only
// default resource and the cascade root. If the default set grows to include a
// genuinely paginated resource, that's fine — this test only fails if a default
// resource maps to a custom (:method) endpoint that still received pagination
// params, which is the exact failure mode F0 fixed.
func TestDefaultSyncRootIsNonPaginated(t *testing.T) {
	for _, resource := range defaultSyncResources() {
		path, err := syncResourcePath(resource)
		if err != nil {
			t.Fatalf("syncResourcePath(%q) returned error: %v", resource, err)
		}
		// Google Ads custom methods use a ":verb" suffix on the collection
		// path (e.g. /v22/customers:listAccessibleCustomers) and bind query
		// params strictly. Any such default resource must be non-paginated.
		if containsColonMethod(path) && !nonPaginatedResources[resource] {
			t.Fatalf("default resource %q maps to custom-method path %q but is not marked non-paginated; sync will send pagination params the API rejects with HTTP 400", resource, path)
		}
	}
}

// containsColonMethod reports whether a path uses the REST custom-method
// shape "<collection>:<verb>" in its final segment.
func containsColonMethod(path string) bool {
	lastSlash := -1
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			lastSlash = i
		}
	}
	for i := lastSlash + 1; i < len(path); i++ {
		if path[i] == ':' {
			return true
		}
	}
	return false
}
