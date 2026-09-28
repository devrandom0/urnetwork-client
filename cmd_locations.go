package main

import (
	"context"
	"fmt"
	"time"

	"github.com/docopt/docopt-go"

	"github.com/devrandom0/urnetwork-client/internal/config"
	"github.com/devrandom0/urnetwork-client/internal/urapi"
)

func cmdLocations(ctx context.Context, opts docopt.Opts) error {
	apiURL := config.StringOr(opts, "--api_url", config.DefaultAPIURL)
	q := config.StringOr(opts, "--query", "")
	jwtOpt, _ := opts.String("--jwt")
	jwt, err := loadJWT(jwtOpt)
	if err != nil {
		return err
	}

	qCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	var res *urapi.LocationsResult
	if q == "" || q == "*" || q == "country:*" || q == "region:*" || q == "group:*" {
		httpRes, err := urapi.ProviderLocations(qCtx, apiURL, jwt)
		if err != nil {
			return err
		}
		res = httpRes
	} else {
		if httpRes, err := urapi.FindLocations(qCtx, apiURL, jwt, q); err == nil && httpRes != nil {
			res = httpRes
		}
		if res == nil || (len(res.Groups) == 0 && len(res.Locations) == 0) {
			hasSpecs, fbRes := urapi.LocationsFallback(qCtx, apiURL, jwt, q)
			if fbRes != nil {
				res = fbRes
			}
			if !hasSpecs && (res == nil || (len(res.Groups) == 0 && len(res.Locations) == 0)) {
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
