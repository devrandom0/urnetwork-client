//go:build darwin || linux

package netcfg

import "net"

// lookupIP is swapped in tests so endpoint bypass needs no real DNS.
var lookupIP = net.LookupIP
