package urapi_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/devrandom0/urnetwork-client/internal/auth"
	"github.com/devrandom0/urnetwork-client/internal/config"
	"github.com/devrandom0/urnetwork-client/internal/urapi"
)

// Integration test (opt-in): requires URNETWORK_TEST_INTEGRATION=1 and a valid JWT in URNETWORK_JWT or ~/.urnetwork/jwt
func TestIntegration_FindLocations_And_FindProviders(t *testing.T) {
	if os.Getenv("URNETWORK_TEST_INTEGRATION") != "1" {
		t.Skip("integration test disabled; set URNETWORK_TEST_INTEGRATION=1 to enable")
	}
	apiURL := config.DefaultAPIURL
	jwt, err := auth.Load(os.Getenv("URNETWORK_JWT"))
	if err != nil {
		t.Skipf("no jwt available: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := urapi.FindLocations(ctx, apiURL, jwt, "country:*"); err != nil {
		t.Fatalf("find-locations failed: %v", err)
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel2()
	specs := urapi.SelectProviders(ctx2, apiURL, jwt, config.LocationConfig{})
	if _, err := urapi.FindProviders(ctx2, apiURL, jwt, specs, 1, "quality"); err != nil {
		t.Fatalf("FindProviders2 failed: %v", err)
	}
}
