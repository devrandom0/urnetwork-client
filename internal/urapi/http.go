package urapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/urnetwork/connect"

	"github.com/devrandom0/urnetwork-client/internal/config"
)

// httpDoer is the minimal interface satisfied by *http.Client.
// Tests can substitute any implementation (e.g., httptest.Server's client).
type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

const (
	apiHTTPTimeout       = 30 * time.Second
	maxAPIResponseBytes  = 4 << 20
	maxAPIErrorBodyBytes = 1 << 10
)

// defaultHTTPClient is used for all API HTTP calls.
// Replace in tests to avoid real network requests.
var defaultHTTPClient httpDoer = &http.Client{Timeout: apiHTTPTimeout, CheckRedirect: checkAPIRedirect}

const maxAPIRedirects = 10

// checkAPIRedirect applies the endpoint URL rules to redirect targets, because the Go client
// resends the Authorization header on same-host redirects, including https to http downgrades.
func checkAPIRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxAPIRedirects {
		return fmt.Errorf("stopped after %d redirects", maxAPIRedirects)
	}
	return config.ValidateEndpointURL("redirect", req.URL.String(), "https", "http")
}

func doAPIRequest(req *http.Request, out any, what string) error {
	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, maxAPIErrorBodyBytes))
		return fmt.Errorf("%s http %d: %s", what, resp.StatusCode, string(data))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAPIResponseBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxAPIResponseBytes {
		return fmt.Errorf("%s: response exceeds %d bytes", what, maxAPIResponseBytes)
	}
	return json.Unmarshal(data, out)
}

// Minimal structures matching the API responses we need
type findLocationsHTTPArgs struct {
	Query               string  `json:"query,omitempty"`
	MaxDistanceFraction float64 `json:"max_distance_fraction,omitempty"`
	EnableMaxDistance   bool    `json:"enable_max_distance_fraction,omitempty"`
}

type LocationsResult struct {
	Specs  []*connect.ProviderSpec `json:"specs"`
	Groups []struct {
		LocationGroupID string `json:"location_group_id"`
		Name            string `json:"name"`
		ProviderCount   int    `json:"provider_count"`
		Promoted        bool   `json:"promoted"`
	} `json:"groups"`
	Locations []struct {
		LocationID        string `json:"location_id"`
		LocationType      string `json:"location_type"`
		Name              string `json:"name"`
		Region            string `json:"region"`
		RegionLocationID  string `json:"region_location_id"`
		Country           string `json:"country"`
		CountryCode       string `json:"country_code"`
		CountryLocationID string `json:"country_location_id"`
		ProviderCount     int    `json:"provider_count"`
	} `json:"locations"`
}

func FindLocations(ctx context.Context, apiURL, jwt, q string) (*LocationsResult, error) {
	body := findLocationsHTTPArgs{Query: q}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(apiURL, "/")+"/network/find-locations", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(jwt) != "" {
		req.Header.Set("Authorization", "Bearer "+jwt)
	}
	var out LocationsResult
	if err := doAPIRequest(req, &out, "find-locations"); err != nil {
		return nil, err
	}
	return &out, nil
}

func ProviderLocations(ctx context.Context, apiURL, jwt string) (*LocationsResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(apiURL, "/")+"/network/provider-locations", nil)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(jwt) != "" {
		req.Header.Set("Authorization", "Bearer "+jwt)
	}
	var out LocationsResult
	if err := doAPIRequest(req, &out, "provider-locations"); err != nil {
		return nil, err
	}
	return &out, nil
}
