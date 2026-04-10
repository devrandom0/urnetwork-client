package main

import (
	"context"
	"fmt"
	"time"

	"github.com/docopt/docopt-go"
	"github.com/urnetwork/connect"
)

func cmdLocations(opts docopt.Opts) {
	apiUrl := getStringOr(opts, "--api_url", DefaultApiUrl)
	q := getStringOr(opts, "--query", "")
	jwtOpt, _ := opts.String("--jwt")
	jwt, err := loadJWT(jwtOpt)
	if err != nil {
		fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	strat := connect.NewClientStrategyWithDefaults(ctx)
	api := connect.NewBringYourApi(ctx, strat, apiUrl)
	api.SetByJwt(jwt)

	var res *findLocationsHTTPResult
	if q == "" || q == "*" || q == "country:*" || q == "region:*" || q == "group:*" {
		httpRes, err := httpProviderLocations(ctx, apiUrl, jwt)
		if err != nil {
			fatal(err)
		}
		res = httpRes
	} else {
		if httpRes, err := httpFindLocations(ctx, apiUrl, jwt, q); err == nil && httpRes != nil {
			res = httpRes
		}
		if res == nil || (len(res.Groups) == 0 && len(res.Locations) == 0) {
			fbSpecs, fbRes := filterLocationsFallback(ctx, strat, apiUrl, jwt, q)
			if fbRes != nil {
				res = fbRes
			}
			if len(fbSpecs) == 0 && (res == nil || (len(res.Groups) == 0 && len(res.Locations) == 0)) {
				fmt.Println("no results")
				return
			}
		}
	}
	if res == nil {
		fmt.Println("no results")
		return
	}

	_ = api // used above for SetByJwt; suppress unused warning

	if len(res.Groups) > 0 {
		fmt.Println("Groups:")
		for _, g := range res.Groups {
			fmt.Printf("  %-30s id=%s providers=%d\n", g.Name, g.LocationGroupId, g.ProviderCount)
		}
	}
	if len(res.Locations) > 0 {
		fmt.Println("Locations:")
		for _, l := range res.Locations {
			id := l.LocationId
			if l.CountryLocationId != "" {
				id = l.CountryLocationId
			}
			if l.RegionLocationId != "" {
				id = l.RegionLocationId
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
}
