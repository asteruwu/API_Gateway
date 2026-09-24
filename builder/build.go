package builder

func Build(configDir string) (*Config, error) {
	gateway, platforms, err := Load(configDir)
	if err != nil {
		return nil, err
	}
	if err := Hydrate(&gateway); err != nil {
		return nil, err
	}
	return Compile(gateway, platforms)
}
