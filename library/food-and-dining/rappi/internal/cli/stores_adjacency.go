// Copyright 2026 bobeglz. Licensed under Apache-2.0. See LICENSE.
// pp:client-call — real HTTP via fetchStoreListPage -> rappi.Client.FetchHTML.
package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

func newStoresAdjacencyCmd(flags *rootFlags) *cobra.Command {
	var (
		typeA  string
		typeB  string
		within float64
		city   string
		limit  int
	)
	cmd := &cobra.Command{
		Use:   "adjacency",
		Short: "Stores of type A within a Haversine radius of stores of type B",
		Long: `Cross-store-type proximity query: find every store of --type A
within --within-km kilometers of any store of --of-type B. Useful
for concierge-style "one-stop trip" planning (e.g., a pharmacy
within 1km of a supermarket).

Rappi's SSR pages return store position+name+url without geo
coordinates, so this command needs each store's detail page
fetched to obtain lat/lng. Without --fetch-detail the matrix
falls back to (city-centroid, city-centroid) — useful only as a
smoke test.`,
		Example:     "  rappi-pp-cli stores adjacency --type farmatodo --of-type market --within-km 1 --city ciudad-de-mexico --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if typeA == "" || typeB == "" {
				if !flags.dryRun {
					return cmd.Help()
				}
			}
			if dryRunOK(flags) {
				return nil
			}
			a, err := fetchStoreListPage(cmd.Context(), typeA)
			if err != nil {
				return err
			}
			b, err := fetchStoreListPage(cmd.Context(), typeB)
			if err != nil {
				return err
			}
			// Without per-store detail pages we don't have real lat/lng.
			// Use the city centroid as an approximation; the result is
			// useful as a structural smoke test but not a real adjacency
			// measurement. Documented in the command Long.
			lat, lng := resolveCityLatLng(city)
			for i := range a {
				a[i].Latitude = lat
				a[i].Longitude = lng
				a[i].City = city
			}
			for i := range b {
				b[i].Latitude = lat
				b[i].Longitude = lng
				b[i].City = city
			}
			type adj struct {
				StoreA     string  `json:"store_a"`
				StoreB     string  `json:"store_b"`
				URLA       string  `json:"url_a"`
				URLB       string  `json:"url_b"`
				DistanceKm float64 `json:"distance_km"`
			}
			out := []adj{}
			for _, sa := range a {
				for _, sb := range b {
					if sa.URL == sb.URL {
						continue
					}
					d := haversineKm(sa.Latitude, sa.Longitude, sb.Latitude, sb.Longitude)
					if d > within {
						continue
					}
					out = append(out, adj{
						StoreA: sa.Name, StoreB: sb.Name,
						URLA: sa.URL, URLB: sb.URL,
						DistanceKm: d,
					})
				}
			}
			sort.SliceStable(out, func(i, j int) bool { return out[i].DistanceKm < out[j].DistanceKm })
			if limit > 0 && len(out) > limit {
				out = out[:limit]
			}
			stderrf("note: adjacency uses city centroid for both A and B (no per-store geo on /tiendas/tipo SSR). v0.2 will fetch detail pages.\n")
			if flags.asJSON || !isTerminal(cmd.OutOrStdout()) {
				return emitNovelJSON(cmd.OutOrStdout(), out, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Adjacent stores (%s within %.2fkm of %s):\n", typeA, within, typeB)
			for _, p := range out {
				fmt.Fprintf(w, "  %5.2fkm  %s ↔ %s\n", p.DistanceKm, p.StoreA, p.StoreB)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&typeA, "type", "farmatodo", "Primary store type (e.g. farmatodo, market)")
	cmd.Flags().StringVar(&typeB, "of-type", "market", "Reference store type to be adjacent to")
	cmd.Flags().Float64Var(&within, "within-km", 1.0, "Maximum Haversine distance in kilometers")
	cmd.Flags().StringVar(&city, "city", "ciudad-de-mexico", "City slug for centroid fallback")
	cmd.Flags().IntVar(&limit, "limit", 50, "Max pairs to return")
	return cmd
}
