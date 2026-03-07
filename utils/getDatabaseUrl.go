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
