package builder

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	gatewayConfig  = "gateway.yaml"
	platformConfig = "platform.yaml"
)

func Load(dir string) (GatewayConfig, []PlatformConfig, error) {
	var gf gatewayFile
	if err := loadYAML(filepath.Join(dir, gatewayConfig), &gf); err != nil {
		return GatewayConfig{}, nil, fmt.Errorf("load gateway: %w", err)
	}

	var pf platformFile
	if err := loadYAML(filepath.Join(dir, platformConfig), &pf); err != nil {
		return GatewayConfig{}, nil, fmt.Errorf("load platform: %w", err)
	}

	return gf.Gateway, pf.Platforms, nil
}

func loadYAML(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, out)
}
