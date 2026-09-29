package utils

import (
	"fmt"
	"net/url"
)

// GetDatabaseURL constructs a PostgreSQL connection string from environment variables.
// It provides sensible defaults for local development.
func GetDatabaseURL() string {
	return buildDatabaseURL(GetEnv("DB_HOST", "localhost"), GetEnv("DB_PORT", "5432"))
}

// GetDatabaseURLForPrefix constructs a PostgreSQL connection string for a specific phase service,
// reading the per-phase DB_HOST_<PREFIX> and DB_PORT_<PREFIX> environment variables (e.g.
// DB_HOST_ASSESSMENT, DB_PORT_ASSESSMENT). defaultPort is used as the local-development fallback
// port for that phase. All other variables (DB_USER, DB_PASSWORD, DB_NAME, SSL_MODE, DB_TIMEZONE)
// are shared with GetDatabaseURL.
func GetDatabaseURLForPrefix(envPrefix, defaultPort string) string {
	return buildDatabaseURL(GetEnv("DB_HOST_"+envPrefix, "localhost"), GetEnv("DB_PORT_"+envPrefix, defaultPort))
}

// buildDatabaseURL uses net/url so a credential containing a reserved character (/, ?, #, %,
// space) is percent-escaped rather than producing a URL that pgx and migrate reject.
func buildDatabaseURL(host, port string) string {
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(GetEnv("DB_USER", "prompt-postgres"), GetEnv("DB_PASSWORD", "prompt-postgres")),
		Host:   fmt.Sprintf("%s:%s", host, port),
		Path:   "/" + GetEnv("DB_NAME", "prompt"),
	}
	query := u.Query()
	query.Set("sslmode", GetEnv("SSL_MODE", "disable"))
	query.Set("TimeZone", GetEnv("DB_TIMEZONE", "Europe/Berlin"))
	u.RawQuery = query.Encode()
	return u.String()
}
