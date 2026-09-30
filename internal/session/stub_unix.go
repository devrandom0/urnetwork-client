//go:build darwin || linux

package session

import "github.com/devrandom0/urnetwork-client/internal/dnsstub"

func startDNSStub(o dnsstub.Options) (stubServer, error) { return dnsstub.Start(o) }
