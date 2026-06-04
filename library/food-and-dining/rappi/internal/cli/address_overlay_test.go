// Copyright 2026 bobe and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestApplyActiveAddressToFlags_DecisionMatrix(t *testing.T) {
	lat := 19.36
	lng := -99.17
	tests := []struct {
		name           string
		command        func() *cobra.Command
		store          *addressStore
		setup          func(*cobra.Command)
		flags          rootFlags
		wantCity       string
		wantLat        string
		wantLng        string
		wantCityChange bool
		wantNote       bool
		wantWarning    bool
	}{
		{
			name:     "near shaped command fills city lat lng",
			command:  newOverlayNearTestCmd,
			store:    testAddressStore(Address{Label: "casa", City: "ciudad-de-mexico", Latitude: &lat, Longitude: &lng}, "casa"),
			wantCity: "ciudad-de-mexico", wantLat: "19.36", wantLng: "-99.17", wantCityChange: true, wantNote: true,
		},
		{
			name:     "city only address fills only city",
			command:  newOverlayNearTestCmd,
			store:    testAddressStore(Address{Label: "casa", City: "ciudad-de-mexico"}, "casa"),
			wantCity: "ciudad-de-mexico", wantLat: "0", wantLng: "0", wantCityChange: true, wantNote: true,
		},
		{
			name:     "city only command gets city without lat lng",
			command:  newOverlayListCityTestCmd,
			store:    testAddressStore(Address{Label: "casa", City: "monterrey", Latitude: &lat, Longitude: &lng}, "casa"),
			wantCity: "monterrey", wantCityChange: true, wantNote: true,
		},
		{
			name:     "adjacency default city is overridden",
			command:  newOverlayAdjacencyTestCmd,
			store:    testAddressStore(Address{Label: "casa", City: "monterrey"}, "casa"),
			wantCity: "monterrey", wantCityChange: true, wantNote: true,
		},
		{
			name:    "explicit city skips entirely",
			command: newOverlayNearTestCmd,
			store:   testAddressStore(Address{Label: "casa", City: "ciudad-de-mexico", Latitude: &lat, Longitude: &lng}, "casa"),
			setup: func(cmd *cobra.Command) {
				_ = cmd.Flags().Set("city", "guadalajara")
			},
			wantCity: "guadalajara", wantLat: "0", wantLng: "0", wantCityChange: true,
		},
		{
			name:    "explicit lat skips entirely",
			command: newOverlayNearTestCmd,
			store:   testAddressStore(Address{Label: "casa", City: "ciudad-de-mexico", Latitude: &lat, Longitude: &lng}, "casa"),
			setup: func(cmd *cobra.Command) {
				_ = cmd.Flags().Set("lat", "20")
			},
			wantCity: "", wantLat: "20", wantLng: "0",
		},
		{
			name:    "profile city skips entirely",
			command: newOverlayNearTestCmd,
			store:   testAddressStore(Address{Label: "casa", City: "ciudad-de-mexico", Latitude: &lat, Longitude: &lng}, "casa"),
			setup: func(cmd *cobra.Command) {
				_ = ApplyProfileToFlags(cmd, &Profile{Name: "p", Values: map[string]string{"city": "puebla"}})
			},
			flags:    rootFlags{profileName: "p"},
			wantCity: "puebla", wantLat: "0", wantLng: "0",
		},
		{
			name:     "no active address is noop",
			command:  newOverlayNearTestCmd,
			store:    emptyAddressStore(),
			wantCity: "", wantLat: "0", wantLng: "0",
		},
		{
			name:    "flagset without city is noop",
			command: newOverlaySyncTestCmd,
			store:   testAddressStore(Address{Label: "casa", City: "ciudad-de-mexico", Latitude: &lat, Longitude: &lng}, "casa"),
		},
		{
			name:     "address subtree excluded",
			command:  func() *cobra.Command { return newOverlayNestedTestCmd("address", newOverlayNearTestCmd()) },
			store:    testAddressStore(Address{Label: "casa", City: "ciudad-de-mexico", Latitude: &lat, Longitude: &lng}, "casa"),
			wantCity: "", wantLat: "0", wantLng: "0",
		},
		{
			name:     "profile subtree excluded",
			command:  func() *cobra.Command { return newOverlayNestedTestCmd("profile", newOverlayNearTestCmd()) },
			store:    testAddressStore(Address{Label: "casa", City: "ciudad-de-mexico", Latitude: &lat, Longitude: &lng}, "casa"),
			wantCity: "", wantLat: "0", wantLng: "0",
		},
		{
			name:     "dangling active pointer warns and noops",
			command:  newOverlayNearTestCmd,
			store:    &addressStore{Addresses: map[string]Address{}, Active: "missing"},
			wantCity: "", wantLat: "0", wantLng: "0", wantWarning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			if tt.flags.profileName != "" {
				if err := saveProfileStore(&profileStore{Profiles: map[string]Profile{
					"p": {Name: "p", Values: map[string]string{"city": "puebla"}},
				}}); err != nil {
					t.Fatalf("save profile store: %v", err)
				}
			}
			if tt.store != nil {
				if err := saveAddressStore(tt.store); err != nil {
					t.Fatalf("save address store: %v", err)
				}
			}
			cmd := tt.command()
			if tt.setup != nil {
				tt.setup(cmd)
			}
			stderr := captureStderr(t, func() {
				ApplyActiveAddressToFlags(cmd, &tt.flags)
			})
			if got := flagValue(cmd, "city"); got != tt.wantCity {
				t.Fatalf("city = %q, want %q", got, tt.wantCity)
			}
			if tt.wantLat != "" {
				if got := flagValue(cmd, "lat"); got != tt.wantLat {
					t.Fatalf("lat = %q, want %q", got, tt.wantLat)
				}
			}
			if tt.wantLng != "" {
				if got := flagValue(cmd, "lng"); got != tt.wantLng {
					t.Fatalf("lng = %q, want %q", got, tt.wantLng)
				}
			}
			if cityFlag := cmd.Flags().Lookup("city"); cityFlag != nil && cityFlag.Changed != tt.wantCityChange {
				t.Fatalf("city Changed = %v, want %v", cityFlag.Changed, tt.wantCityChange)
			}
			if tt.wantNote && !strings.Contains(stderr, `note: active address "casa" applied`) {
				t.Fatalf("stderr missing overlay note: %q", stderr)
			}
			if !tt.wantNote && strings.Contains(stderr, "active address") && !tt.wantWarning {
				t.Fatalf("unexpected overlay note: %q", stderr)
			}
			if tt.wantWarning && !strings.Contains(stderr, "warning: active address") {
				t.Fatalf("stderr missing warning: %q", stderr)
			}
		})
	}
}

func TestApplyActiveAddressToFlags_CorruptStoreDegradesWithWarning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".rappi-pp-cli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, addressesFileName), []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := newOverlayNearTestCmd()
	stderr := captureStderr(t, func() {
		ApplyActiveAddressToFlags(cmd, &rootFlags{})
	})
	if got := flagValue(cmd, "city"); got != "" {
		t.Fatalf("city = %q, want empty", got)
	}
	if !strings.Contains(stderr, "warning: active address store could not be loaded") {
		t.Fatalf("stderr missing corrupt-store warning: %q", stderr)
	}
}

func TestApplyActiveAddressToFlags_NotesAgentAndFetchDetailGuidance(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	lat := 19.36
	lng := -99.17
	if err := saveAddressStore(testAddressStore(Address{Label: "casa", City: "ciudad-de-mexico", Latitude: &lat, Longitude: &lng}, "casa")); err != nil {
		t.Fatal(err)
	}
	cmd := newOverlayNearTestCmd()
	stderr := captureStderr(t, func() {
		ApplyActiveAddressToFlags(cmd, &rootFlags{agent: true})
	})
	if !strings.Contains(stderr, "add --fetch-detail for proximity results") {
		t.Fatalf("stderr missing fetch-detail guidance: %q", stderr)
	}
}

func TestApplyActiveAddressToFlags_DryRunPathDoesNotBlockCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	lat := 19.36
	lng := -99.17
	if err := saveAddressStore(testAddressStore(Address{Label: "casa", City: "ciudad-de-mexico", Latitude: &lat, Longitude: &lng}, "casa")); err != nil {
		t.Fatal(err)
	}

	var flags rootFlags
	root := newRootCmd(&flags)
	root.SetArgs([]string{"restaurants", "near", "--dry-run", "--agent"})
	var out bytes.Buffer
	root.SetOut(&out)
	var err error
	_ = captureStderr(t, func() {
		err = root.Execute()
	})
	if err != nil {
		t.Fatalf("dry-run execute: %v", err)
	}
}

func TestAnnotateAddressOverlayResult(t *testing.T) {
	cmd := newOverlayNearTestCmd()
	cmd.Annotations = map[string]string{
		addressOverlayLabelAnnotation: "casa",
		addressOverlayCityAnnotation:  "ciudad-de-mexico",
	}
	result := map[string]any{}
	annotateAddressOverlayResult(cmd, result)
	got, ok := result["active_address"].(map[string]string)
	if !ok {
		t.Fatalf("active_address missing or wrong type: %#v", result["active_address"])
	}
	if got["label"] != "casa" || got["city"] != "ciudad-de-mexico" {
		t.Fatalf("active_address = %#v", got)
	}
}

func TestAnnotateAddressOverlayResult_NoOverlay(t *testing.T) {
	// A command with no overlay annotations must NOT produce an active_address key.
	cmd := newOverlayNearTestCmd()
	// cmd.Annotations is nil — no overlay was applied.
	result := map[string]any{}
	annotateAddressOverlayResult(cmd, result)
	if _, exists := result["active_address"]; exists {
		t.Fatalf("active_address should not be present when no overlay annotations set, got %#v", result["active_address"])
	}
}

func TestAnnotateAddressOverlayResult_ComposeWithStore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	lat := 19.36
	lng := -99.17
	store := testAddressStore(Address{Label: "casa", City: "ciudad-de-mexico", Latitude: &lat, Longitude: &lng}, "casa")
	if err := saveAddressStore(store); err != nil {
		t.Fatalf("save address store: %v", err)
	}

	cmd := newOverlayNearTestCmd()
	var flags rootFlags
	_ = captureStderr(t, func() {
		ApplyActiveAddressToFlags(cmd, &flags)
	})

	result := map[string]any{}
	annotateAddressOverlayResult(cmd, result)

	aa, ok := result["active_address"].(map[string]string)
	if !ok {
		t.Fatalf("active_address missing or wrong type: %#v", result["active_address"])
	}
	if aa["label"] != "casa" {
		t.Fatalf("active_address label = %q, want casa", aa["label"])
	}
	if aa["city"] != "ciudad-de-mexico" {
		t.Fatalf("active_address city = %q, want ciudad-de-mexico", aa["city"])
	}
}

func newOverlayNearTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "near"}
	cmd.Flags().String("city", "", "")
	cmd.Flags().Float64("lat", 0, "")
	cmd.Flags().Float64("lng", 0, "")
	cmd.Flags().Bool("fetch-detail", false, "")
	return cmd
}

func newOverlayListCityTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "list-city"}
	cmd.Flags().String("city", "", "")
	return cmd
}

func newOverlayAdjacencyTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "adjacency"}
	cmd.Flags().String("city", "ciudad-de-mexico", "")
	return cmd
}

func newOverlaySyncTestCmd() *cobra.Command {
	return &cobra.Command{Use: "sync"}
}

func newOverlayNestedTestCmd(parentName string, child *cobra.Command) *cobra.Command {
	root := &cobra.Command{Use: "rappi-pp-cli"}
	parent := &cobra.Command{Use: parentName}
	parent.AddCommand(child)
	root.AddCommand(parent)
	return child
}

func testAddressStore(addr Address, active string) *addressStore {
	return &addressStore{Addresses: map[string]Address{addr.Label: addr}, Active: active}
}

func flagValue(cmd *cobra.Command, name string) string {
	flag := cmd.Flags().Lookup(name)
	if flag == nil {
		return ""
	}
	return flag.Value.String()
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	fn()
	_ = w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	return buf.String()
}
