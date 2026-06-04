// Copyright 2026 bobe and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddressStore_RoundTripOverwriteAndActiveDelete(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	lat := 19.36
	lng := -99.17
	store := emptyAddressStore()
	store.upsertAddress(Address{Label: "casa", City: "ciudad-de-mexico", Latitude: &lat, Longitude: &lng})
	store.Active = "casa"
	if err := saveAddressStore(store); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := loadAddressStore()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Active != "casa" {
		t.Fatalf("active = %q, want casa", loaded.Active)
	}
	if got := loaded.Addresses["casa"]; got.Label != "casa" || got.City != "ciudad-de-mexico" || got.Latitude == nil || *got.Latitude != lat || got.Longitude == nil || *got.Longitude != lng {
		t.Fatalf("loaded address mismatch: %#v", got)
	}

	newLat := 20.1
	newLng := -100.2
	loaded.upsertAddress(Address{Label: "casa", City: "queretaro", Latitude: &newLat, Longitude: &newLng})
	if err := saveAddressStore(loaded); err != nil {
		t.Fatalf("save after overwrite: %v", err)
	}
	overwritten, err := loadAddressStore()
	if err != nil {
		t.Fatalf("load after overwrite: %v", err)
	}
	if overwritten.Addresses["casa"].City != "queretaro" {
		t.Fatalf("overwritten city = %q, want queretaro", overwritten.Addresses["casa"].City)
	}
	if overwritten.Addresses["casa"].Latitude == nil || *overwritten.Addresses["casa"].Latitude != newLat {
		t.Fatalf("overwritten lat mismatch: %#v", overwritten.Addresses["casa"])
	}
	if overwritten.Addresses["casa"].Longitude == nil || *overwritten.Addresses["casa"].Longitude != newLng {
		t.Fatalf("overwritten lng mismatch: %#v", overwritten.Addresses["casa"])
	}
	if err := overwritten.deleteAddress("casa"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	loaded = overwritten
	if loaded.Active != "" {
		t.Fatalf("delete active left Active=%q", loaded.Active)
	}
	if err := saveAddressStore(loaded); err != nil {
		t.Fatalf("save after delete: %v", err)
	}
	reloaded, err := loadAddressStore()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(reloaded.Addresses) != 0 || reloaded.Active != "" {
		t.Fatalf("reloaded after delete = %#v", reloaded)
	}
}

func TestAddressStore_MissingEmptyAndCityOnlyRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store, err := loadAddressStore()
	if err != nil {
		t.Fatalf("missing load: %v", err)
	}
	if len(store.Addresses) != 0 {
		t.Fatalf("missing store addresses = %d, want 0", len(store.Addresses))
	}
	store.upsertAddress(Address{Label: "oficina", City: "monterrey"})
	if err := saveAddressStore(store); err != nil {
		t.Fatalf("save city-only: %v", err)
	}
	loaded, err := loadAddressStore()
	if err != nil {
		t.Fatalf("load city-only: %v", err)
	}
	addr := loaded.Addresses["oficina"]
	if addr.Latitude != nil || addr.Longitude != nil {
		t.Fatalf("city-only coords should stay nil, got lat=%v lng=%v", addr.Latitude, addr.Longitude)
	}
}

func TestAddressStore_CorruptJSONReturnsParseError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".rappi-pp-cli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, addressesFileName), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := loadAddressStore()
	if err == nil || !strings.Contains(err.Error(), "parsing addresses") {
		t.Fatalf("corrupt load err = %v, want parsing addresses", err)
	}
}

func TestValidateAddress(t *testing.T) {
	lat := 19.36
	lng := -99.17
	tests := []struct {
		name        string
		addr        Address
		wantErr     string
		wantWarning string
	}{
		{name: "empty label", addr: Address{Label: "", City: "ciudad-de-mexico"}, wantErr: "label is required"},
		{name: "whitespace label", addr: Address{Label: "   ", City: "ciudad-de-mexico"}, wantErr: "label is required"},
		{name: "missing city", addr: Address{Label: "casa"}, wantErr: "city is required"},
		{name: "malformed city", addr: Address{Label: "casa", City: "Ciudad de México"}, wantErr: "malformed"},
		{name: "unknown city with coords", addr: Address{Label: "casa", City: "torreon", Latitude: &lat, Longitude: &lng}, wantWarning: "not in the baked city list"},
		{name: "unknown city without coords", addr: Address{Label: "casa", City: "torreon"}, wantWarning: "CDMX default centroid"},
		{name: "lat out of range", addr: Address{Label: "casa", City: "ciudad-de-mexico", Latitude: ptrFloat(200), Longitude: &lng}, wantErr: "latitude"},
		{name: "lng out of range", addr: Address{Label: "casa", City: "ciudad-de-mexico", Latitude: &lat, Longitude: ptrFloat(200)}, wantErr: "longitude"},
		{name: "zero zero rejected", addr: Address{Label: "casa", City: "ciudad-de-mexico", Latitude: ptrFloat(0), Longitude: ptrFloat(0)}, wantErr: "(0,0)"},
		{name: "partial coords rejected", addr: Address{Label: "casa", City: "ciudad-de-mexico", Latitude: &lat}, wantErr: "together"},
		{name: "reserved label", addr: Address{Label: "current", City: "ciudad-de-mexico"}, wantErr: "reserved"},
		{name: "forbidden label chars", addr: Address{Label: "bad/name", City: "ciudad-de-mexico"}, wantErr: "reserved characters"},
		{name: "valid", addr: Address{Label: "casa", City: "ciudad-de-mexico", Latitude: &lat, Longitude: &lng}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warning, err := validateAddress(tt.addr)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if tt.wantWarning != "" && !strings.Contains(warning, tt.wantWarning) {
				t.Fatalf("warning = %q, want containing %q", warning, tt.wantWarning)
			}
			if tt.wantWarning == "" && warning != "" {
				t.Fatalf("warning = %q, want empty", warning)
			}
		})
	}
}

func ptrFloat(v float64) *float64 {
	return &v
}

func TestBuildAgentContext_IncludesAddresses(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	lat := 19.36
	lng := -99.17
	if err := saveAddressStore(testAddressStore(Address{Label: "casa", City: "ciudad-de-mexico", Latitude: &lat, Longitude: &lng}, "casa")); err != nil {
		t.Fatal(err)
	}
	var flags rootFlags
	ctx := buildAgentContext(newRootCmd(&flags))
	if len(ctx.AvailableAddresses) != 1 || ctx.AvailableAddresses[0] != "casa" {
		t.Fatalf("available addresses = %#v, want [casa]", ctx.AvailableAddresses)
	}
	if ctx.ActiveAddress != "casa" {
		t.Fatalf("active address = %q, want casa", ctx.ActiveAddress)
	}
}
