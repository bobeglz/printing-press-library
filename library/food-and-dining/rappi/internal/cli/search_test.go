// Copyright 2026 bobe and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"encoding/json"
	"testing"
)

// asObj decodes a JSON object for assertions, failing the test if it isn't one.
func asObj(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("expected a JSON object, got error %v for %s", err, raw)
	}
	return m
}

func TestExpandSnapshotItems_PreservesProvenance(t *testing.T) {
	snapshot := json.RawMessage(`{
		"city": "mexico-df",
		"category": "pizza",
		"taken_at": "2026-06-01T12:00:00Z",
		"items": [
			{"name": "Pizza Hut", "url": "https://example.com/ph"},
			{"name": "Sushi Bar", "url": "https://example.com/sb"}
		]
	}`)

	// Query matches a single item by its own fields; provenance must ride along.
	out := expandSnapshotItems([]json.RawMessage{snapshot}, "pizza")
	if len(out) != 1 {
		t.Fatalf("want 1 expanded item, got %d", len(out))
	}
	item := asObj(t, out[0])
	if item["name"] != "Pizza Hut" {
		t.Errorf("wrong item expanded: %v", item["name"])
	}
	snap, ok := item["_snapshot"].(map[string]any)
	if !ok {
		t.Fatalf("expanded item missing _snapshot provenance: %v", item)
	}
	for key, want := range map[string]string{
		"city":     "mexico-df",
		"category": "pizza",
		"taken_at": "2026-06-01T12:00:00Z",
	} {
		if snap[key] != want {
			t.Errorf("_snapshot[%q] = %v, want %q", key, snap[key], want)
		}
	}
	if _, leaked := snap["items"]; leaked {
		t.Errorf("_snapshot must not include the items array")
	}
}

func TestExpandSnapshotItems_EmptyQueryExpandsAllWithProvenance(t *testing.T) {
	snapshot := json.RawMessage(`{"city":"mexico-df","taken_at":"2026-06-01T12:00:00Z","items":[{"name":"A"},{"name":"B"}]}`)
	out := expandSnapshotItems([]json.RawMessage{snapshot}, "")
	if len(out) != 2 {
		t.Fatalf("empty query should expand all items, got %d", len(out))
	}
	for i, raw := range out {
		item := asObj(t, raw)
		snap, ok := item["_snapshot"].(map[string]any)
		if !ok {
			t.Fatalf("item %d missing _snapshot: %v", i, item)
		}
		if snap["city"] != "mexico-df" {
			t.Errorf("item %d _snapshot.city = %v, want mexico-df", i, snap["city"])
		}
	}
}

func TestExpandSnapshotItems_ItemFieldsNotClobbered(t *testing.T) {
	// The item carries its own "city"; provenance must not overwrite it, and the
	// snapshot's city must remain reachable under _snapshot.
	snapshot := json.RawMessage(`{"city":"mexico-df","taken_at":"t","items":[{"name":"Place","city":"guadalajara"}]}`)
	out := expandSnapshotItems([]json.RawMessage{snapshot}, "place")
	if len(out) != 1 {
		t.Fatalf("want 1 item, got %d", len(out))
	}
	item := asObj(t, out[0])
	if item["city"] != "guadalajara" {
		t.Errorf("item's own city was clobbered: got %v, want guadalajara", item["city"])
	}
	snap := item["_snapshot"].(map[string]any)
	if snap["city"] != "mexico-df" {
		t.Errorf("_snapshot.city = %v, want mexico-df", snap["city"])
	}
}

func TestExpandSnapshotItems_FutureParentKeyPreserved(t *testing.T) {
	// A top-level key the code doesn't know about must still be carried — the fix
	// preserves provenance structurally, not by an enumerated allow-list.
	snapshot := json.RawMessage(`{"city":"mty","region":"norte","items":[{"name":"X"}]}`)
	out := expandSnapshotItems([]json.RawMessage{snapshot}, "")
	if len(out) != 1 {
		t.Fatalf("want 1 item, got %d", len(out))
	}
	snap := asObj(t, out[0])["_snapshot"].(map[string]any)
	if snap["region"] != "norte" {
		t.Errorf("future parent key 'region' not preserved: %v", snap)
	}
}

func TestExpandSnapshotItems_NonSnapshotPassesThroughUnchanged(t *testing.T) {
	// Records without an "items" array are returned byte-for-byte.
	record := json.RawMessage(`{"name":"Solo Restaurant","city":"mexico-df"}`)
	out := expandSnapshotItems([]json.RawMessage{record}, "solo")
	if len(out) != 1 {
		t.Fatalf("want 1 record, got %d", len(out))
	}
	if string(out[0]) != string(record) {
		t.Errorf("non-snapshot record was mutated:\n got  %s\n want %s", out[0], record)
	}
}

func TestExpandSnapshotItems_UnparseableRecordPassesThrough(t *testing.T) {
	bad := json.RawMessage(`not json{`)
	out := expandSnapshotItems([]json.RawMessage{bad}, "x")
	if len(out) != 1 || string(out[0]) != string(bad) {
		t.Errorf("unparseable record should pass through unchanged, got %v", out)
	}
}

func TestExpandSnapshotItems_SnapshotWithoutOtherKeysAddsNoProvenance(t *testing.T) {
	// A snapshot whose only top-level key is "items" has no provenance to add.
	snapshot := json.RawMessage(`{"items":[{"name":"A"}]}`)
	out := expandSnapshotItems([]json.RawMessage{snapshot}, "")
	if len(out) != 1 {
		t.Fatalf("want 1 item, got %d", len(out))
	}
	if _, has := asObj(t, out[0])["_snapshot"]; has {
		t.Errorf("no provenance should be added when the snapshot has no parent fields")
	}
}

func TestExpandSnapshotItems_QueryMatchingNoItemsDropsSnapshot(t *testing.T) {
	snapshot := json.RawMessage(`{"city":"mexico-df","items":[{"name":"A"},{"name":"B"}]}`)
	out := expandSnapshotItems([]json.RawMessage{snapshot}, "zzz-no-match")
	if len(out) != 0 {
		t.Errorf("snapshot with no matching items should contribute no rows, got %d", len(out))
	}
}

func TestStampSnapshotProvenance_DoesNotClobberExistingSnapshot(t *testing.T) {
	item := json.RawMessage(`{"name":"X","_snapshot":{"foo":"bar"}}`)
	prov := map[string]json.RawMessage{"city": json.RawMessage(`"mexico-df"`)}
	out := stampSnapshotProvenance(item, prov)
	snap := asObj(t, out)["_snapshot"].(map[string]any)
	if snap["foo"] != "bar" || len(snap) != 1 {
		t.Errorf("existing _snapshot was clobbered: %v", snap)
	}
}

func TestStampSnapshotProvenance_NonObjectItemUnchanged(t *testing.T) {
	item := json.RawMessage(`"just-a-string"`)
	prov := map[string]json.RawMessage{"city": json.RawMessage(`"mexico-df"`)}
	if out := stampSnapshotProvenance(item, prov); string(out) != string(item) {
		t.Errorf("non-object item should be returned unchanged, got %s", out)
	}
}

func TestStampSnapshotProvenance_EmptyProvenanceUnchanged(t *testing.T) {
	item := json.RawMessage(`{"name":"X"}`)
	if out := stampSnapshotProvenance(item, nil); string(out) != string(item) {
		t.Errorf("empty provenance should return the item unchanged, got %s", out)
	}
}
