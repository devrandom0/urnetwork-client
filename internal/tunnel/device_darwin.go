//go:build darwin

package tunnel

import (
	"fmt"
	"strings"

	"github.com/songgao/water"

	"github.com/devrandom0/urnetwork-client/internal/logx"
)

// Open creates a utun device named name and falls back to an auto-assigned utun name.
func Open(name string) (Device, error) {
	cfg := water.Config{DeviceType: water.TUN}
	cfg.Name = name
	dev, err := water.New(cfg)
	if err != nil {
		if strings.TrimSpace(cfg.Name) != "" {
			logx.Warn("failed to create %s (%v); retrying with auto utun name\n", cfg.Name, err)
			cfg.Name = ""
			dev, err = water.New(cfg)
		}
		if err != nil {
			return nil, fmt.Errorf("create utun failed: %w", err)
		}
	}
	return dev, nil
}
