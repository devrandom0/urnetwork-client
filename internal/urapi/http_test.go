package urapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHttpFindLocationsAndProviderLocations(t *testing.T) {
	// Fake server
	mux := http.NewServeMux()
	mux.HandleFunc("/network/find-locations", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST")
		}
		if ah := r.Header.Get("Authorization"); !strings.HasPrefix(ah, "Bearer ") {
			t.Fatalf("missing bearer header")
		}
		_ = json.NewEncoder(w).Encode(LocationsResult{})
	})
	mux.HandleFunc("/network/provider-locations", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET")
		}
		if ah := r.Header.Get("Authorization"); !strings.HasPrefix(ah, "Bearer ") {
			t.Fatalf("missing bearer header")
		}
		_ = json.NewEncoder(w).Encode(LocationsResult{})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx := context.Background()
	if _, err := FindLocations(ctx, srv.URL, "token", "country:Germany"); err != nil {
		t.Fatalf("FindLocations error: %v", err)
	}
	if _, err := ProviderLocations(ctx, srv.URL, "token"); err != nil {
		t.Fatalf("ProviderLocations error: %v", err)
	}
}

func useTestServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	old := defaultHTTPClient
	defaultHTTPClient = srv.Client()
	t.Cleanup(func() { defaultHTTPClient = old })
	return srv
}

func TestDefaultHTTPClientHasTimeout(t *testing.T) {
	c, ok := defaultHTTPClient.(*http.Client)
	if !ok {
		t.Fatalf("defaultHTTPClient is %T, want *http.Client", defaultHTTPClient)
	}
	if c == http.DefaultClient || c.Timeout != 30*time.Second {
		t.Fatalf("want a dedicated client with a 30s timeout, got timeout %s", c.Timeout)
	}
}

func TestHttpFindLocations_RejectsOversizedBody(t *testing.T) {
	srv := useTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"groups":[],"pad":"`)
		_, _ = w.Write(bytes.Repeat([]byte("x"), maxAPIResponseBytes))
		_, _ = io.WriteString(w, `"}`)
	})
	_, err := FindLocations(context.Background(), srv.URL, "", "x")
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("err = %v, want a size limit error", err)
	}
}

func TestHttpProviderLocations_TruncatesErrorBody(t *testing.T) {
	srv := useTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(bytes.Repeat([]byte("e"), 1<<20))
	})
	_, err := ProviderLocations(context.Background(), srv.URL, "")
	if err == nil || len(err.Error()) > 2048 {
		t.Fatalf("error must exist and stay short, got %d bytes", len(err.Error()))
	}
}

func TestHttpFindLocations_DecodesSmallBody(t *testing.T) {
	srv := useTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"locations": []map[string]string{{"name": "Berlin"}}})
	})
	res, err := FindLocations(context.Background(), srv.URL, "", "x")
	if err != nil || len(res.Locations) != 1 || res.Locations[0].Name != "Berlin" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func redirectingServer(t *testing.T, location string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			_ = json.NewEncoder(w).Encode(LocationsResult{})
			return
		}
		http.Redirect(w, r, location, http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDefaultHTTPClient_RejectsCleartextRedirect(t *testing.T) {
	srv := redirectingServer(t, "http://api.example.com/network/provider-locations")
	_, err := ProviderLocations(context.Background(), srv.URL, "token")
	if err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("err = %v; want a refused redirect to a cleartext non-loopback URL", err)
	}
}

func TestDefaultHTTPClient_AllowsLoopbackAndHTTPSRedirects(t *testing.T) {
	srv := redirectingServer(t, "/final")
	if _, err := ProviderLocations(context.Background(), srv.URL, "token"); err != nil {
		t.Fatalf("loopback redirect refused: %v", err)
	}
	req, _ := http.NewRequest(http.MethodGet, "https://api.example.com/x", nil)
	if err := checkAPIRedirect(req, nil); err != nil {
		t.Fatalf("https redirect refused: %v", err)
	}
}
