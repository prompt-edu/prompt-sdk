package utils

import "fmt"

const defaultDBPassword = "prompt-postgres"

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

func buildDatabaseURL(host, port string) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s&TimeZone=%s",
		GetEnv("DB_USER", "prompt-postgres"),
		databasePassword(),
		host,
		port,
		GetEnv("DB_NAME", "prompt"),
		GetEnv("SSL_MODE", "disable"),
		GetEnv("DB_TIMEZONE", "Europe/Berlin"),
	)
}

// databasePassword is the single source for the password, so the connection string and the
// mask applied to migration output cannot drift apart and leak the credential.
func databasePassword() string {
	return GetEnv("DB_PASSWORD", defaultDBPassword)
}
