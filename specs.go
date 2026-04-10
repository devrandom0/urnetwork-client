package main

import (
	"context"
	"strings"

	"github.com/urnetwork/connect"
)

// buildProviderSpecs constructs ProviderSpecs from a LocationConfig.
// Priority: LocationID / LocationGroupID → LocationQuery (with HTTP fallback) → BestAvailable.
// It also creates and returns the ClientStrategy needed by the VPN generator.
func buildProviderSpecs(ctx context.Context, apiURL, jwt string, loc LocationConfig) (*connect.ClientStrategy, []*connect.ProviderSpec) {
	strat := connect.NewClientStrategyWithDefaults(ctx)
	specs := []*connect.ProviderSpec{}

	if loc.LocationID != "" {
		if l, err := connect.ParseId(loc.LocationID); err == nil {
			specs = append(specs, &connect.ProviderSpec{LocationId: &l})
		}
	}
	if loc.LocationGroupID != "" {
		if lg, err := connect.ParseId(loc.LocationGroupID); err == nil {
			specs = append(specs, &connect.ProviderSpec{LocationGroupId: &lg})
		}
	}
	if len(specs) == 0 {
		if q := loc.LocationQuery; q != "" {
			if httpRes, err := httpFindLocations(ctx, apiURL, jwt, q); err == nil && httpRes != nil && len(httpRes.Specs) > 0 {
				specs = httpRes.Specs
				logInfo("using %d specs from location query: %s\n", len(specs), q)
			}
			if len(specs) == 0 {
				if fb := findSpecsByQueryFallback(ctx, apiURL, jwt, q); len(fb) > 0 {
					specs = fb
					logInfo("using %d specs from provider-locations (fallback) for: %s\n", len(specs), q)
				}
			}
		}
	}
	if len(specs) == 0 {
		specs = []*connect.ProviderSpec{{BestAvailable: true}}
	}
	return strat, specs
}

// findSpecsByQueryFallback queries /network/provider-locations and builds ProviderSpecs
// by client-side filtering for queries like 'country:Germany', 'region:Europe', 'group:West'.
func findSpecsByQueryFallback(ctx context.Context, apiURL, jwt, q string) []*connect.ProviderSpec {
	_, res := filterLocationsFallback(ctx, apiURL, jwt, q)
	if res == nil {
		return nil
	}
	key, val, ok := parseKV(q)
	if !ok {
		key = "name"
		val = strings.TrimSpace(q)
	}
	key = strings.ToLower(strings.TrimSpace(key))
	valNorm := strings.ToLower(strings.TrimSpace(val))

	seen := map[string]bool{}
	specs := []*connect.ProviderSpec{}

	if key == "group" || key == "name" {
		for _, g := range res.Groups {
			if matchValueFold(g.Name, valNorm) {
				if id, err := connect.ParseId(g.LocationGroupID); err == nil && !seen[id.String()] {
					seen[id.String()] = true
					specs = append(specs, &connect.ProviderSpec{LocationGroupId: &id})
				}
			}
		}
	}
	for _, l := range res.Locations {
		var match bool
		switch key {
		case "country":
			match = matchValueFold(l.Country, valNorm) || matchValueFold(l.Name, valNorm)
		case "country_code":
			match = strings.EqualFold(strings.TrimSpace(l.CountryCode), val)
		case "region":
			match = matchValueFold(l.Region, valNorm) || (l.LocationType == "region" && matchValueFold(l.Name, valNorm))
		case "name":
			match = matchValueFold(l.Name, valNorm)
		default:
			match = matchValueFold(l.Name, valNorm) || matchValueFold(l.Country, valNorm) || matchValueFold(l.Region, valNorm)
		}
		if !match {
			continue
		}
		idStr := l.LocationID
		if key == "country" && l.CountryLocationID != "" {
			idStr = l.CountryLocationID
		}
		if key == "region" && l.RegionLocationID != "" {
			idStr = l.RegionLocationID
		}
		if id, err := connect.ParseId(idStr); err == nil && !seen[id.String()] {
			seen[id.String()] = true
			specs = append(specs, &connect.ProviderSpec{LocationId: &id})
		}
	}
	return specs
}

// filterLocationsFallback fetches /network/provider-locations and returns a filtered result
// plus convenience ProviderSpecs for the given query q.
func filterLocationsFallback(ctx context.Context, apiURL, jwt, q string) ([]*connect.ProviderSpec, *findLocationsHTTPResult) {
	res, err := httpProviderLocations(ctx, apiURL, jwt)
	if err != nil || res == nil {
		return nil, nil
	}
	key, val, ok := parseKV(q)
	if !ok {
		key = "name"
		val = strings.TrimSpace(q)
	}
	key = strings.ToLower(strings.TrimSpace(key))
	valNorm := strings.ToLower(strings.TrimSpace(val))

	out := &findLocationsHTTPResult{}
	if key == "group" || key == "name" {
		for _, g := range res.Groups {
			if matchValueFold(g.Name, valNorm) {
				out.Groups = append(out.Groups, g)
			}
		}
	}
	for _, l := range res.Locations {
		var match bool
		switch key {
		case "country":
			match = matchValueFold(l.Country, valNorm) || matchValueFold(l.Name, valNorm)
		case "country_code":
			match = strings.EqualFold(strings.TrimSpace(l.CountryCode), val)
		case "region":
			match = matchValueFold(l.Region, valNorm) || (l.LocationType == "region" && matchValueFold(l.Name, valNorm))
		case "name":
			match = matchValueFold(l.Name, valNorm)
		default:
			match = matchValueFold(l.Name, valNorm) || matchValueFold(l.Country, valNorm) || matchValueFold(l.Region, valNorm)
		}
		if match {
			out.Locations = append(out.Locations, l)
		}
	}

	seen := map[string]bool{}
	specs := []*connect.ProviderSpec{}
	for _, g := range out.Groups {
		if id, err := connect.ParseId(g.LocationGroupID); err == nil && !seen[id.String()] {
			seen[id.String()] = true
			specs = append(specs, &connect.ProviderSpec{LocationGroupId: &id})
		}
	}
	for _, l := range out.Locations {
		idStr := l.LocationID
		if key == "country" && l.CountryLocationID != "" {
			idStr = l.CountryLocationID
		}
		if key == "region" && l.RegionLocationID != "" {
			idStr = l.RegionLocationID
		}
		if id, err := connect.ParseId(idStr); err == nil && !seen[id.String()] {
			seen[id.String()] = true
			specs = append(specs, &connect.ProviderSpec{LocationId: &id})
		}
	}
	return specs, out
}

// parseKV splits a "key:value" query string. Returns ok=false when there is no colon.
func parseKV(q string) (string, string, bool) {
	if q == "" {
		return "", "", false
	}
	i := strings.Index(q, ":")
	if i < 0 {
		return "", "", false
	}
	return q[:i], q[i+1:], true
}

// matchValueFold reports whether s contains valNorm (case-insensitive).
// The special value "*" matches any non-empty s.
func matchValueFold(s, valNorm string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	if valNorm == "*" {
		return s != ""
	}
	return strings.Contains(s, valNorm)
}
