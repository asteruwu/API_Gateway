package builder

import (
	"fmt"
	"path/filepath"

	"API_Gateway/pkg/tools"
)

const (
	gatewayConfig  = "gateway.yaml"
	platformConfig = "platform.yaml"
)

func Load(dir string) (GatewayConfig, []PlatformConfig, error) {
	var gf gatewayFile
	if err := tools.LoadYAML(filepath.Join(dir, gatewayConfig), &gf); err != nil {
		return GatewayConfig{}, nil, fmt.Errorf("load gateway: %w", err)
	}

	var pf platformFile
	if err := tools.LoadYAML(filepath.Join(dir, platformConfig), &pf); err != nil {
		return GatewayConfig{}, nil, fmt.Errorf("load platform: %w", err)
	}

	return gf.Gateway, pf.Platforms, nil
}
