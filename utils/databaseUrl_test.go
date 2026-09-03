package utils

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetDatabaseURLForPrefix(t *testing.T) {
	t.Setenv("DB_USER", "prompt-postgres")
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("DB_NAME", "prompt")
	t.Setenv("DB_HOST_ASSESSMENT", "db.example.com")
	t.Setenv("DB_PORT_ASSESSMENT", "6543")

	url := GetDatabaseURLForPrefix("ASSESSMENT", "5435")
	require.Equal(t, "postgres://prompt-postgres:secret@db.example.com:6543/prompt?sslmode=disable&TimeZone=Europe/Berlin", url)
}

func TestGetDatabaseURLForPrefixFallsBackToDefaultPort(t *testing.T) {
	t.Setenv("DB_PASSWORD", "secret")
	// DB_HOST_INTERVIEW and DB_PORT_INTERVIEW intentionally unset.
	url := GetDatabaseURLForPrefix("INTERVIEW", "5438")
	require.True(t, strings.Contains(url, "@localhost:5438/"), "expected localhost + default port, got %q", url)
}

func TestSanitizeDatabaseURL(t *testing.T) {
	url := "postgres://prompt-postgres:s3cr3t@localhost:5432/prompt?sslmode=disable"
	require.Equal(t, "postgres://prompt-postgres:***@localhost:5432/prompt?sslmode=disable", SanitizeDatabaseURL(url, "s3cr3t"))
	require.Equal(t, url, SanitizeDatabaseURL(url, ""), "empty password must be a no-op")
}
