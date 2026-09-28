//go:build linux

package tunnel

import (
	"fmt"

	"github.com/songgao/water"
)

// Open creates the named TUN device.
func Open(name string) (Device, error) {
	cfg := water.Config{DeviceType: water.TUN}
	cfg.Name = name
	dev, err := water.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("create TUN failed: %w", err)
	}
	return dev, nil
}
