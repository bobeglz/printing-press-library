// Copyright 2026 bobe and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mvanhorn/printing-press-library/library/food-and-dining/rappi/internal/source/rappi"
)

// Address is a named delivery location. Latitude and longitude are nullable
// because existing geo commands use (0,0) as their unset sentinel.
type Address struct {
	Label     string   `json:"label"`
	City      string   `json:"city"`
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
}

type addressStore struct {
	Addresses map[string]Address `json:"addresses"`
	Active    string             `json:"active,omitempty"`
}

const addressesFileName = "addresses.json"

var citySlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func addressStorePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	if strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("resolving home dir: empty home")
	}
	return filepath.Join(home, ".rappi-pp-cli", addressesFileName), nil
}

func loadAddressStore() (*addressStore, error) {
	p, err := addressStorePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return emptyAddressStore(), nil
		}
		return nil, fmt.Errorf("reading addresses: %w", err)
	}
	var s addressStore
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing addresses: %w", err)
	}
	if s.Addresses == nil {
		s.Addresses = map[string]Address{}
	}
	return &s, nil
}

func saveAddressStore(s *addressStore) error {
	p, err := addressStorePath()
	if err != nil {
		return err
	}
	// The state dir is created on the write path only; loads treat a missing
	// dir or file as an empty store.
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("creating state dir: %w", err)
	}
	if s == nil {
		s = emptyAddressStore()
	}
	if s.Addresses == nil {
		s.Addresses = map[string]Address{}
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling addresses: %w", err)
	}
	// Write to a uniquely-named temp file in the same dir, then atomically
	// rename it into place. A unique name (rather than a fixed "<path>.tmp")
	// stops two concurrent CLI invocations from racing over a shared temp
	// file, so each writer's bytes land intact and its rename is independent.
	// Distinct concurrent updates still resolve last-writer-wins — there is no
	// lock spanning load-modify-save — which is acceptable for a single-user CLI.
	tmp, err := os.CreateTemp(filepath.Dir(p), ".addresses-*.tmp")
	if err != nil {
		return fmt.Errorf("writing addresses: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing addresses: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing addresses: %w", err)
	}
	if err := os.Rename(tmpPath, p); err != nil {
		return fmt.Errorf("writing addresses: %w", err)
	}
	return nil
}

func emptyAddressStore() *addressStore {
	return &addressStore{Addresses: map[string]Address{}}
}

// upsertAddress inserts or replaces an already-validated address. Validation
// happens exactly once, in the save command (validateAddress), before any
// disk access.
func (s *addressStore) upsertAddress(addr Address) {
	if s.Addresses == nil {
		s.Addresses = map[string]Address{}
	}
	s.Addresses[addr.Label] = addr
}

func (s *addressStore) get(label string) (Address, error) {
	addr, ok := s.Addresses[label]
	if !ok {
		return Address{}, notFoundErr(fmt.Errorf("address %q not found", label))
	}
	return addr, nil
}

func (s *addressStore) deleteAddress(label string) error {
	if _, err := s.get(label); err != nil {
		return err
	}
	delete(s.Addresses, label)
	if s.Active == label {
		s.Active = ""
	}
	return nil
}

// validateAddress enforces a two-tier contract: the returned error is a hard
// rejection (the address must not be persisted), while the returned warning
// string is a non-blocking advisory the caller prints to stderr (e.g.,
// well-formed city slugs outside the baked list). A new validation rule is
// either a rejection (error) or an advisory (warning) — never both.
func validateAddress(addr Address) (string, error) {
	label := strings.TrimSpace(addr.Label)
	if label == "" {
		return "", fmt.Errorf("address label is required")
	}
	if label != addr.Label || strings.ContainsAny(label, `/\: `) {
		return "", fmt.Errorf("address label %q contains reserved characters", addr.Label)
	}
	if isReservedAddressLabel(label) {
		return "", fmt.Errorf("address label %q is reserved", label)
	}

	city := strings.TrimSpace(addr.City)
	if city == "" {
		return "", fmt.Errorf("address city is required; run 'rappi-pp-cli cities list' for known city slugs")
	}
	if city != addr.City || !citySlugPattern.MatchString(city) {
		return "", fmt.Errorf("city slug %q is malformed; run 'rappi-pp-cli cities list' for known city slugs", addr.City)
	}

	if addr.Latitude == nil && addr.Longitude != nil || addr.Latitude != nil && addr.Longitude == nil {
		return "", fmt.Errorf("latitude and longitude must be saved together")
	}
	if addr.Latitude != nil {
		if *addr.Latitude < -90 || *addr.Latitude > 90 {
			return "", fmt.Errorf("latitude %.6f out of range [-90, 90]", *addr.Latitude)
		}
		if *addr.Longitude < -180 || *addr.Longitude > 180 {
			return "", fmt.Errorf("longitude %.6f out of range [-180, 180]", *addr.Longitude)
		}
		if *addr.Latitude == 0 && *addr.Longitude == 0 {
			return "", fmt.Errorf("address coordinates cannot be exact (0,0); omit coords when unknown")
		}
	}

	if rappi.CityBySlug(city) == nil {
		// The "CDMX default centroid" wording mirrors resolveCityLatLng's
		// silent fallback (novel_helpers.go) — keep the two in sync.
		if addr.Latitude == nil {
			return fmt.Sprintf("warning: city slug %q is not in the baked city list; proximity commands will fall back to the CDMX default centroid until you save latitude and longitude", city), nil
		}
		return fmt.Sprintf("warning: city slug %q is not in the baked city list; live Rappi URLs may still work, but run 'rappi-pp-cli cities list' for known city slugs", city), nil
	}
	return "", nil
}

// ListAddressNames returns saved address labels sorted alphabetically. Used
// by the agent-context introspection surface (mirrors ListProfileNames).
func ListAddressNames() []string {
	s, err := loadAddressStore()
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(s.Addresses))
	for name := range s.Addresses {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ActiveAddressLabel returns the active address label, or "" when none is
// set or the store is unreadable.
func ActiveAddressLabel() string {
	s, err := loadAddressStore()
	if err != nil {
		return ""
	}
	return s.Active
}

func isReservedAddressLabel(label string) bool {
	switch strings.ToLower(label) {
	case "save", "list", "show", "use", "current", "delete":
		return true
	default:
		return false
	}
}
