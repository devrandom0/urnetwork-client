package main

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strings"

    "github.com/urnetwork/connect"
)

// Minimal structures matching the API responses we need
type findLocationsHTTPArgs struct {
    Query               string  `json:"query,omitempty"`
    MaxDistanceFraction float64 `json:"max_distance_fraction,omitempty"`
    EnableMaxDistance   bool    `json:"enable_max_distance_fraction,omitempty"`
}

type findLocationsHTTPResult struct {
    Specs     []*connect.ProviderSpec `json:"specs"`
    Groups    []struct {
        LocationGroupId string `json:"location_group_id"`
        Name            string `json:"name"`
        ProviderCount   int    `json:"provider_count"`
        Promoted        bool   `json:"promoted"`
    } `json:"groups"`
    Locations []struct {
        LocationId        string `json:"location_id"`
        LocationType      string `json:"location_type"`
        Name              string `json:"name"`
        Region            string `json:"region"`
        RegionLocationId  string `json:"region_location_id"`
        Country           string `json:"country"`
        CountryCode       string `json:"country_code"`
        CountryLocationId string `json:"country_location_id"`
        ProviderCount     int    `json:"provider_count"`
    } `json:"locations"`
}

func httpFindLocations(ctx context.Context, apiUrl, jwt, q string) (*findLocationsHTTPResult, error) {
    body := findLocationsHTTPArgs{Query: q}
    b, _ := json.Marshal(body)
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(apiUrl, "/")+"/network/find-locations", bytes.NewReader(b))
    if err != nil { return nil, err }
    req.Header.Set("Content-Type", "application/json")
    if strings.TrimSpace(jwt) != "" { req.Header.Set("Authorization", "Bearer "+jwt) }
    resp, err := http.DefaultClient.Do(req)
    if err != nil { return nil, err }
    defer resp.Body.Close()
    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        data, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("find-locations http %d: %s", resp.StatusCode, string(data))
    }
    var out findLocationsHTTPResult
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil { return nil, err }
    return &out, nil
}

func httpProviderLocations(ctx context.Context, apiUrl, jwt string) (*findLocationsHTTPResult, error) {
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(apiUrl, "/")+"/network/provider-locations", nil)
    if err != nil { return nil, err }
    if strings.TrimSpace(jwt) != "" { req.Header.Set("Authorization", "Bearer "+jwt) }
    resp, err := http.DefaultClient.Do(req)
    if err != nil { return nil, err }
    defer resp.Body.Close()
    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        data, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("provider-locations http %d: %s", resp.StatusCode, string(data))
    }
    var out findLocationsHTTPResult
    if err := json.NewDecoder(resp.Body).Decode(&out); err != nil { return nil, err }
    return &out, nil
}
