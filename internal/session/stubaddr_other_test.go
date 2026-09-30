//go:build !darwin

package session

import (
	"net/netip"
	"testing"

	"github.com/devrandom0/urnetwork-client/internal/config"
)

var wantStubHost = netip.MustParseAddr("10.255.0.2")

func TestPlanDNS_ListensOnTUNAddress(t *testing.T) {
	plan, err := planDNS(config.VPNConfig{IPCIDR: "10.255.0.2/24"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.listen.String() != "10.255.0.2:53" {
		t.Fatalf("stub listens on %s; want the TUN address", plan.listen)
	}
}
