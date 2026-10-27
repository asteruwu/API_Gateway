package builder

func Build(configDir string) (*Config, error) {
	gateway, platforms, err := Load(configDir)
	if err != nil {
		return nil, err
	}

	plan := Calculate(gateway, platforms)

	if err := ValidateWriting(gateway, platforms, plan); err != nil {
		return nil, err
	}
	if err := Hydrate(&gateway, plan.HTTPFilters); err != nil {
		return nil, err
	}
	if err := ValidateRuntime(gateway, plan); err != nil {
		return nil, err
	}

	return Compile(gateway, plan)
}
