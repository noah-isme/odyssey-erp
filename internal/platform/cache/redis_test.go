package cache

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAsynqOptionsSupportsRenderConnectionURL(t *testing.T) {
	options, err := AsynqOptions("redis://render-user:secret@red-example:6379/2")
	require.NoError(t, err)
	require.Equal(t, "red-example:6379", options.Addr)
	require.Equal(t, "render-user", options.Username)
	require.Equal(t, "secret", options.Password)
	require.Equal(t, 2, options.DB)
}

func TestAsynqOptionsSupportsLegacyAddress(t *testing.T) {
	options, err := AsynqOptions("localhost:6379")
	require.NoError(t, err)
	require.Equal(t, "localhost:6379", options.Addr)
}
