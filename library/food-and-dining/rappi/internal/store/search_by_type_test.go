// Copyright 2026 bobe and contributors. Licensed under Apache-2.0. See LICENSE.
package store

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// TestSearchByType_RestrictsToResourceType proves the --type narrowing contract:
// SearchByType returns only rows of the requested resource_type, while the
// unfiltered Search returns matches across every type.
func TestSearchByType_RestrictsToResourceType(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	mustUpsertForSearch(t, s, "restaurant", "r1", `{"name":"Pizza Palace","city":"cdmx"}`)
	mustUpsertForSearch(t, s, "store", "s1", `{"name":"Pizza Mart","city":"cdmx"}`)

	// Scoped to restaurants -> only the restaurant.
	got, err := s.SearchByType("pizza", "restaurant", 10)
	if err != nil {
		t.Fatalf("SearchByType(restaurant): %v", err)
	}
	if len(got) != 1 || !strings.Contains(string(got[0]), "Pizza Palace") {
		t.Fatalf("restaurant search = %s, want only Pizza Palace", got)
	}

	// The same query scoped to stores -> only the store.
	got, err = s.SearchByType("pizza", "store", 10)
	if err != nil {
		t.Fatalf("SearchByType(store): %v", err)
	}
	if len(got) != 1 || !strings.Contains(string(got[0]), "Pizza Mart") {
		t.Fatalf("store search = %s, want only Pizza Mart", got)
	}

	// A type with no rows -> no matches.
	got, err = s.SearchByType("pizza", "cafe", 10)
	if err != nil {
		t.Fatalf("SearchByType(cafe): %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("cafe search = %s, want empty", got)
	}

	// Unfiltered Search still spans both types.
	all, err := s.Search("pizza", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("unfiltered search = %d rows, want 2 (both types)", len(all))
	}
}

func mustUpsertForSearch(t *testing.T, s *Store, resourceType, id, data string) {
	t.Helper()
	if err := s.Upsert(resourceType, id, json.RawMessage(data)); err != nil {
		t.Fatalf("upsert %s/%s: %v", resourceType, id, err)
	}
}
