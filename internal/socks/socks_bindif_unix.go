//go:build unix

package socks

import (
	"fmt"
	"net"
	"runtime"
	"strings"
	"syscall"
)

const (
	darwinIPBoundIF     = 25  // IP_BOUND_IF
	darwinIPv6BoundIF   = 125 // IPV6_BOUND_IF
	linuxSOBindToDevice = 25  // SO_BINDTODEVICE, needs CAP_NET_RAW
)

// bindFDToInterface pins a socket to ifName. Any failure is returned so the caller refuses
// the request; silently continuing would send VPN traffic out the normal default route.
func bindFDToInterface(fd int, network, ifName string) error {
	if strings.TrimSpace(ifName) == "" {
		return nil
	}
	ifi, err := net.InterfaceByName(ifName)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", errBindInterface, ifName, err)
	}
	switch runtime.GOOS {
	case "darwin":
		level, opt := syscall.IPPROTO_IP, darwinIPBoundIF
		if strings.HasSuffix(network, "6") {
			level, opt = syscall.IPPROTO_IPV6, darwinIPv6BoundIF
		}
		err = syscall.SetsockoptInt(fd, level, opt, ifi.Index)
	case "linux":
		err = syscall.SetsockoptString(fd, syscall.SOL_SOCKET, linuxSOBindToDevice, ifName)
	default:
		err = fmt.Errorf("not supported on %s", runtime.GOOS)
	}
	if err != nil {
		return fmt.Errorf("%w: %s: %v", errBindInterface, ifName, err)
	}
	return nil
}
