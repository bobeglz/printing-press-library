// Copyright 2026 bobe and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestSaveAddressStore_ConcurrentWritesNoTempLeak exercises the race-free
// atomic write: many concurrent savers must leave a single valid store file and
// no leftover temp files. A fixed-name "<path>.tmp" would have raced here.
func TestSaveAddressStore_ConcurrentWritesNoTempLeak(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := saveAddressStore(emptyAddressStore()); err != nil {
		t.Fatalf("seed save: %v", err)
	}
	p, err := addressStorePath()
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	dir := filepath.Dir(p)

	const writers = 16
	var wg sync.WaitGroup
	errs := make([]error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := emptyAddressStore()
			s.upsertAddress(Address{Label: "casa", City: "ciudad-de-mexico"})
			s.Active = "casa"
			errs[i] = saveAddressStore(s)
		}(i)
	}
	wg.Wait()

	for i, e := range errs {
		if e != nil {
			t.Fatalf("concurrent writer %d failed: %v", i, e)
		}
	}

	// The persisted file must be valid and complete — no torn write.
	loaded, err := loadAddressStore()
	if err != nil {
		t.Fatalf("load after concurrent writes: %v", err)
	}
	if loaded.Active != "casa" || loaded.Addresses["casa"].City != "ciudad-de-mexico" {
		t.Fatalf("store corrupted by concurrent writes: %#v", loaded)
	}

	// No stray temp files may survive.
	leftovers, err := filepath.Glob(filepath.Join(dir, ".addresses-*.tmp"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(leftovers) != 0 {
		t.Fatalf("leftover temp files after concurrent writes: %v", leftovers)
	}
}

// TestSaveAddressStore_FileMode confirms the persisted store keeps owner-only
// permissions (os.CreateTemp defaults to 0o600, matching the prior WriteFile).
func TestSaveAddressStore_FileMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := saveAddressStore(emptyAddressStore()); err != nil {
		t.Fatalf("save: %v", err)
	}
	p, err := addressStorePath()
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("file mode = %v, want 0600", perm)
	}
}
