package utils

import (
	"net/url"
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
	require.Equal(t, "postgres://prompt-postgres:secret@db.example.com:6543/prompt?TimeZone=Europe%2FBerlin&sslmode=disable", url)
}

func TestGetDatabaseURLForPrefixFallsBackToDefaultPort(t *testing.T) {
	t.Setenv("DB_PASSWORD", "secret")
	// DB_HOST_INTERVIEW and DB_PORT_INTERVIEW intentionally unset.
	url := GetDatabaseURLForPrefix("INTERVIEW", "5438")
	require.True(t, strings.Contains(url, "@localhost:5438/"), "expected localhost + default port, got %q", url)
}

func TestGetDatabaseURLEscapesReservedCharactersInCredentials(t *testing.T) {
	t.Setenv("DB_USER", "prompt user")
	t.Setenv("DB_PASSWORD", "p/ss?w#rd%1")
	t.Setenv("DB_HOST_INTERVIEW", "localhost")
	t.Setenv("DB_PORT_INTERVIEW", "5438")

	parsed, err := url.Parse(GetDatabaseURLForPrefix("INTERVIEW", "5438"))
	require.NoError(t, err, "a password with reserved characters must still yield a parseable URL")
	require.Equal(t, "localhost:5438", parsed.Host)
	require.Equal(t, "prompt user", parsed.User.Username())
	password, _ := parsed.User.Password()
	require.Equal(t, "p/ss?w#rd%1", password)
}

func TestSanitizeDatabaseURL(t *testing.T) {
	require.Equal(t,
		"postgres://prompt-postgres:***@localhost:5432/prompt?sslmode=disable",
		SanitizeDatabaseURL("postgres://prompt-postgres:s3cr3t@localhost:5432/prompt?sslmode=disable"))

	require.Equal(t,
		"error: postgresql://prompt-postgres:***@localhost:5432/prompt failed",
		SanitizeDatabaseURL("error: postgresql://prompt-postgres:p%2Fss@localhost:5432/prompt failed"),
		"an escaped password must be masked too")

	require.Equal(t,
		"postgres://prompt-postgres:***@localhost:5435/prompt",
		SanitizeDatabaseURL("postgres://prompt-postgres:prompt-postgres@localhost:5435/prompt"),
		"a password equal to the user name must not mask the user name")

	require.Equal(t,
		"migration 5432 failed for prompt",
		SanitizeDatabaseURL("migration 5432 failed for prompt"),
		"text without a connection string must be left alone")
}
