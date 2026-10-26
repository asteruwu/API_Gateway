package builder

func Build(configDir string) (*Config, error) {
	gateway, platforms, err := Load(configDir)
	if err != nil {
		return nil, err
	}
	httpActivated := Calculate(gateway, platforms)
	if err := Hydrate(&gateway, httpActivated); err != nil {
		return nil, err
	}
	return Compile(gateway, platforms)
}
