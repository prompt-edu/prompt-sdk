package utils

import "fmt"

// GetDatabaseURL constructs a PostgreSQL connection string from environment variables.
// It provides sensible defaults for local development.
func GetDatabaseURL() string {
	dbUser := GetEnv("DB_USER", "prompt-postgres")
	dbPassword := GetEnv("DB_PASSWORD", "prompt-postgres")
	dbHost := GetEnv("DB_HOST", "localhost")
	dbPort := GetEnv("DB_PORT", "5432")
	dbName := GetEnv("DB_NAME", "prompt")
	sslMode := GetEnv("SSL_MODE", "disable")
	timeZone := GetEnv("DB_TIMEZONE", "Europe/Berlin") // Add a timezone parameter

	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s&TimeZone=%s", dbUser, dbPassword, dbHost, dbPort, dbName, sslMode, timeZone)
}

// GetDatabaseURLForPrefix constructs a PostgreSQL connection string for a specific phase service,
// reading the per-phase DB_HOST_<PREFIX> and DB_PORT_<PREFIX> environment variables (e.g.
// DB_HOST_ASSESSMENT, DB_PORT_ASSESSMENT). defaultPort is used as the local-development fallback
// port for that phase. All other variables (DB_USER, DB_PASSWORD, DB_NAME, SSL_MODE, DB_TIMEZONE)
// are shared with GetDatabaseURL.
func GetDatabaseURLForPrefix(envPrefix, defaultPort string) string {
	dbUser := GetEnv("DB_USER", "prompt-postgres")
	dbPassword := GetEnv("DB_PASSWORD", "prompt-postgres")
	dbHost := GetEnv("DB_HOST_"+envPrefix, "localhost")
	dbPort := GetEnv("DB_PORT_"+envPrefix, defaultPort)
	dbName := GetEnv("DB_NAME", "prompt")
	sslMode := GetEnv("SSL_MODE", "disable")
	timeZone := GetEnv("DB_TIMEZONE", "Europe/Berlin")

	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s&TimeZone=%s", dbUser, dbPassword, dbHost, dbPort, dbName, sslMode, timeZone)
}
