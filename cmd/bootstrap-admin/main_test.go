package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateBootstrapInput(t *testing.T) {
	require.NoError(t, validateBootstrapInput("postgres://db", "admin@example.com", "strong-password"))
	require.Error(t, validateBootstrapInput("", "admin@example.com", "strong-password"))
	require.Error(t, validateBootstrapInput("postgres://db", "invalid", "strong-password"))
	require.Error(t, validateBootstrapInput("postgres://db", "admin@example.com", "short"))
}

func TestAdminPermissionsAreUnique(t *testing.T) {
	seen := make(map[string]struct{})
	for _, permission := range adminPermissions() {
		_, exists := seen[permission]
		require.False(t, exists, "duplicate permission %s", permission)
		seen[permission] = struct{}{}
	}
}
