package utils

import "regexp"

// an escaped password cannot contain "@", "/" or whitespace, so the match stays inside the userinfo
var databaseURLCredential = regexp.MustCompile(`(postgres(?:ql)?://[^:@/\s]+:)[^@/\s]*(@)`)

// SanitizeDatabaseURL masks the password of every PostgreSQL URL in input with "***", so a
// connection string (or tool output echoing one) can be logged without leaking the credential.
// It masks the URL segment rather than the password value, so an escaped password is covered too
// and an unrelated substring that happens to equal the password is not redacted.
func SanitizeDatabaseURL(input string) string {
	return databaseURLCredential.ReplaceAllString(input, "${1}***${2}")
}
