// Copyright 2026 bobe and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	addressOverlayLabelAnnotation = "rappi:active-address-label"
	addressOverlayCityAnnotation  = "rappi:active-address-city"
)

// ApplyActiveAddressToFlags overlays the persisted active address onto geo
// flags when a command did not receive explicit location context.
func ApplyActiveAddressToFlags(cmd *cobra.Command, flags *rootFlags) {
	if cmd == nil || addressOverlayExcluded(cmd) || lookupFlag(cmd, "city") == nil {
		return
	}
	if anyGeoFlagExplicit(cmd, flags) {
		return
	}

	store, err := loadAddressStore()
	if err != nil {
		stderrf("warning: active address store could not be loaded; skipping address overlay: %v\n", err)
		return
	}
	if store.Active == "" {
		return
	}
	addr, ok := store.Addresses[store.Active]
	if !ok {
		stderrf("warning: active address %q was not found in addresses.json; skipping address overlay\n", store.Active)
		return
	}

	cityFlag := lookupFlag(cmd, "city")
	setFlagChanged(cityFlag, addr.City)

	filledLatLng := false
	if addr.Latitude != nil {
		if latFlag := lookupFlag(cmd, "lat"); latFlag != nil {
			setFlagChanged(latFlag, fmt.Sprintf("%.12g", *addr.Latitude))
			filledLatLng = true
		}
	}
	if addr.Longitude != nil {
		if lngFlag := lookupFlag(cmd, "lng"); lngFlag != nil {
			setFlagChanged(lngFlag, fmt.Sprintf("%.12g", *addr.Longitude))
			filledLatLng = true
		}
	}
	ann := ensureAnnotations(cmd)
	ann[addressOverlayLabelAnnotation] = addr.Label
	ann[addressOverlayCityAnnotation] = addr.City

	note := fmt.Sprintf("note: active address %q applied (city %s)", addr.Label, addr.City)
	if filledLatLng {
		// Capability probe, not a per-command list: any command exposing an
		// unchanged fetch-detail flag gets the hint (list pages carry no geo).
		if fetchDetail := lookupFlag(cmd, "fetch-detail"); fetchDetail != nil && !fetchDetail.Changed && fetchDetail.Value.String() == "false" {
			note += "; add --fetch-detail for proximity results"
		}
	}
	stderrf("%s\n", note)
}

func annotateAddressOverlayResult(cmd *cobra.Command, result map[string]any) {
	if cmd == nil || result == nil {
		return
	}
	// cmd.Annotations may be nil; map reads on a nil map return zero values safely.
	label := cmd.Annotations[addressOverlayLabelAnnotation]
	city := cmd.Annotations[addressOverlayCityAnnotation]
	if label == "" {
		return
	}
	result["active_address"] = map[string]string{
		"label": label,
		"city":  city,
	}
}

func addressOverlayExcluded(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		switch c.Name() {
		case "address", "profile", "completion", "help", "version":
			return true
		}
	}
	return false
}

func anyGeoFlagExplicit(cmd *cobra.Command, flags *rootFlags) bool {
	// Load the profile store once per invocation: ApplyProfileToFlags sets
	// values without marking flags Changed, so profile-provided geo context
	// is only detectable here — but it must not cost one disk read per flag.
	var profileValues map[string]string
	if flags != nil && flags.profileName != "" {
		if profile, err := GetProfile(flags.profileName); err == nil && profile != nil {
			profileValues = profile.Values
		}
	}
	for _, name := range []string{"city", "lat", "lng"} {
		flag := lookupFlag(cmd, name)
		if flag == nil {
			continue
		}
		if flag.Changed {
			return true
		}
		if _, ok := profileValues[name]; ok {
			return true
		}
	}
	return false
}

func lookupFlag(cmd *cobra.Command, name string) *pflag.Flag {
	if cmd == nil {
		return nil
	}
	if flag := cmd.Flags().Lookup(name); flag != nil {
		return flag
	}
	return cmd.InheritedFlags().Lookup(name)
}

func setFlagChanged(flag *pflag.Flag, value string) {
	if flag == nil {
		return
	}
	_ = flag.Value.Set(value)
	flag.Changed = true
}

func ensureAnnotations(cmd *cobra.Command) map[string]string {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	return cmd.Annotations
}
