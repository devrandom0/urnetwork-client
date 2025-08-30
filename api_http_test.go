package main

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
)

func TestHttpFindLocationsAndProviderLocations(t *testing.T) {
    // Fake server
    mux := http.NewServeMux()
    mux.HandleFunc("/network/find-locations", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost { t.Fatalf("expected POST") }
        if ah := r.Header.Get("Authorization"); !strings.HasPrefix(ah, "Bearer ") { t.Fatalf("missing bearer header") }
        _ = json.NewEncoder(w).Encode(findLocationsHTTPResult{})
    })
    mux.HandleFunc("/network/provider-locations", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodGet { t.Fatalf("expected GET") }
        if ah := r.Header.Get("Authorization"); !strings.HasPrefix(ah, "Bearer ") { t.Fatalf("missing bearer header") }
        _ = json.NewEncoder(w).Encode(findLocationsHTTPResult{})
    })
    srv := httptest.NewServer(mux)
    defer srv.Close()

    ctx := context.Background()
    if _, err := httpFindLocations(ctx, srv.URL, "token", "country:Germany"); err != nil {
        t.Fatalf("httpFindLocations error: %v", err)
    }
    if _, err := httpProviderLocations(ctx, srv.URL, "token"); err != nil {
        t.Fatalf("httpProviderLocations error: %v", err)
    }
}
