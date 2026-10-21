package builder

var transformerHydrators = map[string]pluginHydrator{
	"testT": hydrateTestTransformer,
}

func hydrateTestTransformer(raw any) (any, error) {
	return TestTransformerConfig{}, nil
}
