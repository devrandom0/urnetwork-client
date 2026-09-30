package safefile

import "testing"

// TestSafefiletestTagCanary has no build tag, so it always compiles, unlike
// store_root_test.go/config_root_test.go which only exist under -tags safefiletest. If a gate,
// CI job or local `go test` invocation drops that tag, those tests are silently skipped rather
// than run; this canary turns that silence into a loud, specific failure instead.
func TestSafefiletestTagCanary(t *testing.T) {
	if !safefiletestEnabled {
		t.Fatal("run tests with -tags safefiletest")
	}
}
