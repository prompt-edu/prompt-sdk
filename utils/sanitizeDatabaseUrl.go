package utils

import "strings"

// SanitizeDatabaseURL masks occurrences of password in input with "***", so a database URL (or
// migration tool output that echoes it) can be logged without leaking the credential. An empty
// password returns input unchanged. It masks the raw password form, matching how GetDatabaseURL /
// GetDatabaseURLForPrefix embed the password verbatim.
func SanitizeDatabaseURL(input, password string) string {
	if password == "" {
		return input
	}
	return strings.ReplaceAll(input, password, "***")
}
