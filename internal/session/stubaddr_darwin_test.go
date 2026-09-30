//go:build darwin

package session

import (
	"net/netip"
	"testing"

	"github.com/devrandom0/urnetwork-client/internal/config"
)

var wantStubHost = netip.MustParseAddr("127.0.0.1")

// On macOS a stub on the TUN address is reached through the TUN: mDNSResponder scopes the
// queries to the interface, so they enter the tunnel instead of the local stub.
func TestPlanDNS_ListensOnLoopbackOnMacOS(t *testing.T) {
	plan, err := planDNS(config.VPNConfig{IPCIDR: "10.255.0.2/24"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.listen.String() != "127.0.0.1:53" {
		t.Fatalf("stub listens on %s; want 127.0.0.1:53", plan.listen)
	}
}
