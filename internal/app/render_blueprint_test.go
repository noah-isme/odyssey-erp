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
			Plan string `yaml:"plan"`
		} `yaml:"services"`
		Databases []struct {
			Name string `yaml:"name"`
			Plan string `yaml:"plan"`
		} `yaml:"databases"`
	}
	require.NoError(t, yaml.Unmarshal(content, &blueprint))
	require.Len(t, blueprint.Services, 1)
	require.Len(t, blueprint.Databases, 1)

	require.Equal(t, "odyssey-web", blueprint.Services[0].Name)
	require.Equal(t, "web", blueprint.Services[0].Type)
	require.Equal(t, "free", blueprint.Services[0].Plan)
	require.Equal(t, "odyssey-postgres", blueprint.Databases[0].Name)
	require.Equal(t, "free", blueprint.Databases[0].Plan)
}
