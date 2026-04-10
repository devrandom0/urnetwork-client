package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/urnetwork/connect"
)

// makeSpecServer spins up an httptest server that serves a /network/find-locations response
// containing the supplied ProviderSpecs. It redirects defaultHTTPClient for the duration
// of the test and restores it on cleanup.
func makeSpecServer(t *testing.T, specs []*connect.ProviderSpec) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/network/find-locations" {
			_ = json.NewEncoder(w).Encode(findLocationsHTTPResult{Specs: specs})
			return
		}
		if r.URL.Path == "/network/provider-locations" {
			_ = json.NewEncoder(w).Encode(findLocationsHTTPResult{})
			return
		}
		http.NotFound(w, r)
	}))
	old := defaultHTTPClient
	defaultHTTPClient = srv.Client()
	t.Cleanup(func() {
		defaultHTTPClient = old
		srv.Close()
	})
	return srv
}

func TestBuildProviderSpecs_BestAvailable(t *testing.T) {
	// No location flags → BestAvailable spec.
	ctx := context.Background()
	_, specs := buildProviderSpecs(ctx, "http://unused", "", LocationConfig{})
	if len(specs) != 1 || !specs[0].BestAvailable {
		t.Fatalf("expected single BestAvailable spec, got %v", specs)
	}
}

func TestBuildProviderSpecs_LocationID(t *testing.T) {
	// LocationID set → single spec with that ID; no HTTP call.
	ctx := context.Background()
	// Use a valid UUID-like ID. The connect library's ParseId should accept it.
	id := "00000000-0000-0000-0000-000000000001"
	_, specs := buildProviderSpecs(ctx, "http://unused", "", LocationConfig{LocationID: id})
	if len(specs) == 0 {
		t.Fatal("expected at least one spec for valid location ID")
	}
	if specs[0].LocationId == nil {
		t.Fatal("expected LocationId to be set")
	}
}

func TestBuildProviderSpecs_LocationGroupID(t *testing.T) {
	// LocationGroupID set → single spec with that group ID; no HTTP call.
	ctx := context.Background()
	id := "00000000-0000-0000-0000-000000000002"
	_, specs := buildProviderSpecs(ctx, "http://unused", "", LocationConfig{LocationGroupID: id})
	if len(specs) == 0 {
		t.Fatal("expected at least one spec for valid location group ID")
	}
	if specs[0].LocationGroupId == nil {
		t.Fatal("expected LocationGroupId to be set")
	}
}

func TestBuildProviderSpecs_LocationQuery_HTTPSuccess(t *testing.T) {
	// LocationQuery with a successful HTTP response → uses returned specs.
	wantID := "00000000-0000-0000-0000-000000000003"
	parsedID, err := connect.ParseId(wantID)
	if err != nil {
		t.Fatalf("ParseId: %v", err)
	}
	returnedSpecs := []*connect.ProviderSpec{{LocationId: &parsedID}}
	srv := makeSpecServer(t, returnedSpecs)

	ctx := context.Background()
	_, specs := buildProviderSpecs(ctx, srv.URL, "", LocationConfig{LocationQuery: "country:Germany"})
	if len(specs) == 0 {
		t.Fatal("expected specs from HTTP response")
	}
	if specs[0].LocationId == nil || specs[0].LocationId.String() != wantID {
		t.Fatalf("unexpected spec: %v", specs[0])
	}
}

func TestBuildProviderSpecs_LocationQuery_HTTPEmpty_FallsBackToBestAvailable(t *testing.T) {
	// LocationQuery with an empty HTTP response and empty fallback → BestAvailable.
	srv := makeSpecServer(t, nil) // server returns empty Specs slice

	ctx := context.Background()
	_, specs := buildProviderSpecs(ctx, srv.URL, "", LocationConfig{LocationQuery: "country:NowhereXYZ"})
	if len(specs) != 1 || !specs[0].BestAvailable {
		t.Fatalf("expected BestAvailable fallback, got %v", specs)
	}
}
