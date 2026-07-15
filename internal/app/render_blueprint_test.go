package app_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestRenderBlueprintParses(t *testing.T) {
	content, err := os.ReadFile("../../render.yaml")
	require.NoError(t, err)

	var blueprint struct {
		Services []struct {
			Name string `yaml:"name"`
			Type string `yaml:"type"`
		} `yaml:"services"`
		Databases []struct {
			Name string `yaml:"name"`
		} `yaml:"databases"`
	}
	require.NoError(t, yaml.Unmarshal(content, &blueprint))
	require.Len(t, blueprint.Services, 4)
	require.Len(t, blueprint.Databases, 1)

	names := make(map[string]struct{}, len(blueprint.Services))
	for _, service := range blueprint.Services {
		require.NotEmpty(t, service.Name)
		require.NotEmpty(t, service.Type)
		names[service.Name] = struct{}{}
	}
	for _, name := range []string{"odyssey-web", "odyssey-worker", "odyssey-gotenberg", "odyssey-keyvalue"} {
		_, ok := names[name]
		require.True(t, ok, "missing Render service %s", name)
	}
	require.Equal(t, "odyssey-postgres", blueprint.Databases[0].Name)
}
