package testutils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConnectReturnsPingError(t *testing.T) {
	conn, err := connect(t.Context(), "postgres://testuser:testpass@127.0.0.1:1/prompt?sslmode=disable")

	require.Error(t, err)
	require.Nil(t, conn)
}
