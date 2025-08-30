package main

import (
    "context"
    "os"
    "testing"
    "time"
    "github.com/urnetwork/connect"
)

// Integration test (opt-in): requires URNETWORK_TEST_INTEGRATION=1 and a valid JWT in URNETWORK_JWT or ~/.urnetwork/jwt
func TestIntegration_FindLocations_And_FindProviders(t *testing.T) {
    if os.Getenv("URNETWORK_TEST_INTEGRATION") != "1" {
        t.Skip("integration test disabled; set URNETWORK_TEST_INTEGRATION=1 to enable")
    }
    apiUrl := DefaultApiUrl
    jwt, err := loadJWT(os.Getenv("URNETWORK_JWT"))
    if err != nil { t.Skipf("no jwt available: %v", err) }

    // locations via http helpers
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    if _, err := httpFindLocations(ctx, apiUrl, jwt, "country:*"); err != nil {
        t.Fatalf("find-locations failed: %v", err)
    }

    // find-providers via API
    ctx2, cancel2 := context.WithTimeout(context.Background(), 20*time.Second)
    defer cancel2()
    strat := connect.NewClientStrategyWithDefaults(ctx2)
    api := connect.NewBringYourApi(ctx2, strat, apiUrl)
    api.SetByJwt(jwt)
    specs := []*connect.ProviderSpec{{BestAvailable: true}}
    if _, err := api.FindProviders2Sync(&connect.FindProviders2Args{Specs: specs, Count: 1, RankMode: "quality"}); err != nil {
        t.Fatalf("FindProviders2 failed: %v", err)
    }
}
