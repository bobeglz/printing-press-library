// Copyright 2026 bobe and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func newAddressCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "address",
		Short: "Manage saved delivery addresses and the active location",
		Long: `Save named delivery addresses, persist one active address, and let
location-aware commands use it when no --city/--lat/--lng flags are set.

  address save <label> --city <slug> [--lat <n> --lng <n>]
  address use <label>       persists the active address for future commands
  address current           shows the active address
  address list              lists saved addresses
  address show <label>      shows one address
  address delete <label>    deletes one address

Unlike 'profile use', 'address use' writes local state: it persists the
active address pointer in ~/.rappi-pp-cli/addresses.json.`,
		Annotations: map[string]string{"mcp:hidden": "true"},
	}
	cmd.AddCommand(newAddressSaveCmd(flags))
	cmd.AddCommand(newAddressListCmd(flags))
	cmd.AddCommand(newAddressShowCmd(flags))
	cmd.AddCommand(newAddressUseCmd(flags))
	cmd.AddCommand(newAddressCurrentCmd(flags))
	cmd.AddCommand(newAddressDeleteCmd(flags))
	return cmd
}

func newAddressSaveCmd(flags *rootFlags) *cobra.Command {
	var city string
	var lat, lng float64
	cmd := &cobra.Command{
		Use:   "save <label>",
		Short: "Save or replace a delivery address",
		Example: `  rappi-pp-cli address save casa --city ciudad-de-mexico --lat 19.36 --lng -99.17
  rappi-pp-cli address save torreon-casa --city torreon`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var latPtr, lngPtr *float64
			if cmd.Flags().Changed("lat") {
				latPtr = &lat
			}
			if cmd.Flags().Changed("lng") {
				lngPtr = &lng
			}
			addr := Address{Label: args[0], City: city, Latitude: latPtr, Longitude: lngPtr}
			warning, err := validateAddress(addr)
			if err != nil {
				return err
			}
			if dryRunOK(flags) {
				if warning != "" {
					stderrf("%s\n", warning)
				}
				return nil
			}
			store, err := loadAddressStore()
			if err != nil {
				return err
			}
			store.upsertAddress(addr)
			if err := saveAddressStore(store); err != nil {
				return err
			}
			if warning != "" {
				stderrf("%s\n", warning)
			}
			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), addr, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "saved address %q (%s)\n", addr.Label, addr.City)
			return nil
		},
	}
	cmd.Flags().StringVar(&city, "city", "", "City slug (required; run 'cities list' for known values)")
	cmd.Flags().Float64Var(&lat, "lat", 0, "Delivery latitude")
	cmd.Flags().Float64Var(&lng, "lng", 0, "Delivery longitude")
	return cmd
}

func newAddressListCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "List saved delivery addresses",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if dryRunOK(flags) {
				return nil
			}
			store, err := loadAddressStore()
			if err != nil {
				return err
			}
			rows := addressListRows(store)
			if flags.asJSON {
				outFlags := *flags
				outFlags.compact = false
				return printJSONFiltered(cmd.OutOrStdout(), rows, &outFlags)
			}
			table := make([][]string, 0, len(rows))
			for _, row := range rows {
				active := ""
				if row.Active {
					active = "*"
				}
				table = append(table, []string{active, row.Label, row.City, formatOptionalFloat(row.Latitude), formatOptionalFloat(row.Longitude)})
			}
			return flags.printTable(cmd, []string{"ACTIVE", "LABEL", "CITY", "LAT", "LNG"}, table)
		},
	}
}

func newAddressShowCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:         "show <label>",
		Short:       "Show a saved delivery address",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			addr, err := getAddress(args[0])
			if err != nil {
				return err
			}
			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), addr, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "address %q:\n", addr.Label)
			fmt.Fprintf(cmd.OutOrStdout(), "  city: %s\n", addr.City)
			if addr.Latitude != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "  lat: %.12g\n", *addr.Latitude)
				fmt.Fprintf(cmd.OutOrStdout(), "  lng: %.12g\n", *addr.Longitude)
			}
			return nil
		},
	}
}

func newAddressUseCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "use <label>",
		Short: "Persist the active delivery address for future commands",
		Long: `Persist the active delivery address pointer. This differs from
'profile use', which only prints profile values for inspection.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			store, err := loadAddressStore()
			if err != nil {
				return err
			}
			addr, err := store.get(args[0])
			if err != nil {
				return err
			}
			store.Active = args[0]
			if err := saveAddressStore(store); err != nil {
				return err
			}
			out := map[string]any{"active": args[0], "address": addr}
			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), out, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "active address set to %q (%s)\n", addr.Label, addr.City)
			return nil
		},
	}
}

func newAddressCurrentCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:         "current",
		Short:       "Show the active delivery address",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if dryRunOK(flags) {
				return nil
			}
			store, err := loadAddressStore()
			if err != nil {
				return err
			}
			var active *Address
			if store.Active != "" {
				if addr, ok := store.Addresses[store.Active]; ok {
					active = &addr
				}
			}
			if flags.asJSON {
				result := map[string]any{"active": nil, "active_label": ""}
				if active != nil {
					result["active"] = *active
					result["active_label"] = store.Active
				}
				return printJSONFiltered(cmd.OutOrStdout(), result, flags)
			}
			if active == nil {
				fmt.Fprintln(cmd.OutOrStdout(), "no active address")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "active address: %s (%s)\n", active.Label, active.City)
			return nil
		},
	}
}

func newAddressDeleteCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <label>",
		Short: "Delete a saved delivery address",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			label := args[0]
			store, err := loadAddressStore()
			if err != nil {
				return err
			}
			if _, err := store.get(label); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return nil
			}
			if !flags.yes {
				fmt.Fprintf(cmd.ErrOrStderr(), "refusing to delete %q without --yes\n", label)
				return fmt.Errorf("confirmation required: pass --yes")
			}
			wasActive := store.Active == label
			if err := store.deleteAddress(label); err != nil {
				return err
			}
			if err := saveAddressStore(store); err != nil {
				return err
			}
			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"deleted": label, "cleared_active": wasActive}, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "deleted address %q\n", label)
			return nil
		},
	}
}

// addressListRow embeds Address so the JSON shape tracks the Address fields
// automatically (label, city, latitude, longitude) plus the active marker.
type addressListRow struct {
	Address
	Active bool `json:"active"`
}

func addressListRows(store *addressStore) []addressListRow {
	labels := make([]string, 0, len(store.Addresses))
	for label := range store.Addresses {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	rows := make([]addressListRow, 0, len(labels))
	for _, label := range labels {
		rows = append(rows, addressListRow{
			Address: store.Addresses[label],
			Active:  label == store.Active,
		})
	}
	return rows
}

func getAddress(label string) (Address, error) {
	store, err := loadAddressStore()
	if err != nil {
		return Address{}, err
	}
	return store.get(label)
}

func formatOptionalFloat(v *float64) string {
	if v == nil {
		return ""
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", *v), "0"), ".")
}
