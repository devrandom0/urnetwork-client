package main

import (
	"context"
	"fmt"
	"time"

	"github.com/docopt/docopt-go"
)

func cmdLocations(ctx context.Context, opts docopt.Opts) error {
	apiURL := getStringOr(opts, "--api_url", DefaultAPIURL)
	q := getStringOr(opts, "--query", "")
	jwtOpt, _ := opts.String("--jwt")
	jwt, err := loadJWT(jwtOpt)
	if err != nil {
		return err
	}

	qCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	var res *findLocationsHTTPResult
	if q == "" || q == "*" || q == "country:*" || q == "region:*" || q == "group:*" {
		httpRes, err := httpProviderLocations(qCtx, apiURL, jwt)
		if err != nil {
			return err
		}
		res = httpRes
	} else {
		if httpRes, err := httpFindLocations(qCtx, apiURL, jwt, q); err == nil && httpRes != nil {
			res = httpRes
		}
		if res == nil || (len(res.Groups) == 0 && len(res.Locations) == 0) {
			fbSpecs, fbRes := filterLocationsFallback(qCtx, apiURL, jwt, q)
			if fbRes != nil {
				res = fbRes
			}
			if len(fbSpecs) == 0 && (res == nil || (len(res.Groups) == 0 && len(res.Locations) == 0)) {
				fmt.Println("no results")
				return nil
			}
		}
	}
	if res == nil {
		fmt.Println("no results")
		return nil
	}

	if len(res.Groups) > 0 {
		fmt.Println("Groups:")
		for _, g := range res.Groups {
			fmt.Printf("  %-30s id=%s providers=%d\n", g.Name, g.LocationGroupID, g.ProviderCount)
		}
	}
	if len(res.Locations) > 0 {
		fmt.Println("Locations:")
		for _, l := range res.Locations {
			id := l.LocationID
			if l.CountryLocationID != "" {
				id = l.CountryLocationID
			}
			if l.RegionLocationID != "" {
				id = l.RegionLocationID
			}
			extra := l.Country
			if l.Region != "" {
				extra = l.Region
			}
			if extra != "" {
				fmt.Printf("  %-18s %-24s id=%s providers=%d\n", l.LocationType, l.Name+" ("+extra+")", id, l.ProviderCount)
			} else {
				fmt.Printf("  %-18s %-24s id=%s providers=%d\n", l.LocationType, l.Name, id, l.ProviderCount)
			}
		}
	}
	return nil
}
