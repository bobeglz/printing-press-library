// Copyright 2026 bobe and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mvanhorn/printing-press-library/library/food-and-dining/rappi/internal/mcp/cobratree"

	"github.com/mark3labs/mcp-go/server"
)

func TestAddressCommands_SaveShowUseCurrentListDelete(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	out, err := runAddressCLI("address", "save", "casa", "--city", "ciudad-de-mexico", "--lat", "19.36", "--lng", "-99.17", "--agent")
	if err != nil {
		t.Fatalf("save casa: %v\n%s", err, out)
	}
	assertJSON(t, out)
	if !jsonContains(t, out, "label", "casa") {
		t.Fatalf("save output missing label: %s", out)
	}

	out, err = runAddressCLI("address", "show", "casa", "--agent")
	if err != nil {
		t.Fatalf("show casa: %v\n%s", err, out)
	}
	if !jsonContains(t, out, "city", "ciudad-de-mexico") || !strings.Contains(out, `"latitude": 19.36`) {
		t.Fatalf("show output mismatch: %s", out)
	}

	out, err = runAddressCLI("address", "save", "oficina", "--city", "monterrey", "--agent")
	if err != nil {
		t.Fatalf("save oficina: %v\n%s", err, out)
	}
	if strings.Contains(out, "latitude") || strings.Contains(out, "longitude") {
		t.Fatalf("city-only address should omit coords: %s", out)
	}

	out, err = runAddressCLI("address", "use", "casa", "--agent")
	if err != nil {
		t.Fatalf("use casa: %v\n%s", err, out)
	}
	if !jsonContains(t, out, "active", "casa") {
		t.Fatalf("use output mismatch: %s", out)
	}

	out, err = runAddressCLI("address", "current", "--agent")
	if err != nil {
		t.Fatalf("current: %v\n%s", err, out)
	}
	if !jsonContains(t, out, "active_label", "casa") {
		t.Fatalf("current output mismatch: %s", out)
	}

	out, err = runAddressCLI("address", "list", "--agent")
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"label": "casa"`) || !strings.Contains(out, `"active": true`) {
		t.Fatalf("list output mismatch: %s", out)
	}

	out, err = runAddressCLI("address", "delete", "casa", "--yes", "--agent")
	if err != nil {
		t.Fatalf("delete casa: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"cleared_active": true`) {
		t.Fatalf("delete output mismatch: %s", out)
	}
	out, err = runAddressCLI("address", "current", "--agent")
	if err != nil {
		t.Fatalf("current after delete: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"active": null`) || !strings.Contains(out, `"active_label": ""`) {
		t.Fatalf("empty current output mismatch: %s", out)
	}
}

func TestAddressCommands_EmptyStates(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	out, err := runAddressCLI("address", "current", "--agent")
	if err != nil {
		t.Fatalf("current empty: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"active": null`) {
		t.Fatalf("current empty output mismatch: %s", out)
	}
	out, err = runAddressCLI("address", "list", "--json")
	if err != nil {
		t.Fatalf("list empty: %v\n%s", err, out)
	}
	if strings.TrimSpace(out) != "[]" {
		t.Fatalf("list empty = %q, want []", out)
	}
}

func TestAddressCommands_ErrorPaths(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, tc := range []struct {
		name         string
		args         []string
		want         string
		wantExitCode int
	}{
		{name: "show missing", args: []string{"address", "show", "missing", "--agent"}, want: `address "missing" not found`, wantExitCode: 3},
		{name: "use missing", args: []string{"address", "use", "missing", "--agent"}, want: `address "missing" not found`, wantExitCode: 3},
		{name: "delete missing", args: []string{"address", "delete", "missing", "--agent"}, want: `address "missing" not found`, wantExitCode: 3},
		{name: "reserved save", args: []string{"address", "save", "current", "--city", "ciudad-de-mexico", "--agent"}, want: "reserved"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runAddressCLI(tc.args...)
			if err == nil {
				t.Fatalf("expected error, got nil output=%s", out)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
			if tc.wantExitCode != 0 && ExitCode(err) != tc.wantExitCode {
				t.Fatalf("exit code = %d, want %d (err=%v)", ExitCode(err), tc.wantExitCode, err)
			}
		})
	}
}

func TestAddressCommands_DeleteNonActive_ClearedActiveFalse(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, err := runAddressCLI("address", "save", "casa", "--city", "ciudad-de-mexico", "--agent"); err != nil {
		t.Fatalf("save casa: %v", err)
	}
	if _, err := runAddressCLI("address", "save", "oficina", "--city", "monterrey", "--agent"); err != nil {
		t.Fatalf("save oficina: %v", err)
	}
	if _, err := runAddressCLI("address", "use", "casa", "--agent"); err != nil {
		t.Fatalf("use casa: %v", err)
	}

	out, err := runAddressCLI("address", "delete", "oficina", "--yes", "--agent")
	if err != nil {
		t.Fatalf("delete oficina: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"cleared_active": false`) {
		t.Fatalf("delete non-active: expected cleared_active false, got: %s", out)
	}

	out, err = runAddressCLI("address", "current", "--agent")
	if err != nil {
		t.Fatalf("current after delete: %v\n%s", err, out)
	}
	if !jsonContains(t, out, "active_label", "casa") {
		t.Fatalf("active should still be casa after deleting oficina, got: %s", out)
	}
}

func TestAddressCommands_DeleteRequiresYesAfterExistenceCheck(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, err := runAddressCLI("address", "save", "casa", "--city", "ciudad-de-mexico", "--agent"); err != nil {
		t.Fatalf("save: %v", err)
	}
	_, err := runAddressCLI("address", "delete", "casa")
	if err == nil || !strings.Contains(err.Error(), "confirmation required") {
		t.Fatalf("delete without yes err = %v, want confirmation required", err)
	}
}

func TestAddressCommands_UnknownCityWarning(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var err error
	stderr := captureStderr(t, func() {
		_, err = runAddressCLI("address", "save", "torreon-casa", "--city", "torreon", "--agent")
	})
	if err != nil {
		t.Fatalf("save unknown city: %v", err)
	}
	if !strings.Contains(stderr, "CDMX default centroid") {
		t.Fatalf("stderr warning = %q, want CDMX fallback", stderr)
	}
}

func TestAddressCommand_MCPHiddenGuard(t *testing.T) {
	root := RootCmd()
	address, _, err := root.Find([]string{"address"})
	if err != nil {
		t.Fatalf("find address: %v", err)
	}
	if address == nil || address.Annotations["mcp:hidden"] != "true" {
		t.Fatalf("address command should be mcp hidden, got %#v", address)
	}
	s := server.NewMCPServer("rappi-test", "0")
	cobratree.RegisterAll(s, root, func() (string, error) { return "rappi-pp-cli", nil })
	for name := range s.ListTools() {
		if strings.HasPrefix(name, "address") {
			t.Fatalf("address command leaked into MCP tool registry as %q", name)
		}
	}
}

func runAddressCLI(args ...string) (string, error) {
	var flags rootFlags
	root := newRootCmd(&flags)
	root.SetArgs(args)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	err := root.Execute()
	return out.String(), err
}

func assertJSON(t *testing.T, text string) {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		t.Fatalf("invalid JSON %q: %v", text, err)
	}
}

func jsonContains(t *testing.T, text, key string, want any) bool {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal([]byte(text), &obj); err != nil {
		t.Fatalf("invalid JSON %q: %v", text, err)
	}
	return obj[key] == want
}

func TestAddressCommandConstructorsCompile(t *testing.T) {
	if newAddressCmd(&rootFlags{}).Name() != "address" {
		t.Fatal("address command name mismatch")
	}
}

func TestAddressShow_HandEditedPartialCoordsDoesNotPanic(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// Simulate a hand-edited addresses.json: latitude set, longitude missing.
	// saveAddressStore does not re-validate, so this partial state loads — and
	// the text-mode show path used to dereference the nil longitude and panic.
	lat := 19.36
	s := emptyAddressStore()
	s.upsertAddress(Address{Label: "casa", City: "ciudad-de-mexico", Latitude: &lat})
	if err := saveAddressStore(s); err != nil {
		t.Fatalf("save: %v", err)
	}
	out, err := runAddressCLI("address", "show", "casa") // text mode (no --agent)
	if err != nil {
		t.Fatalf("show: %v\n%s", err, out)
	}
	if !strings.Contains(out, "city: ciudad-de-mexico") {
		t.Errorf("show output missing city: %s", out)
	}
	if strings.Contains(out, "lat:") || strings.Contains(out, "lng:") {
		t.Errorf("partial coords should be omitted, not printed: %s", out)
	}
}
