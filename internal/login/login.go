// Package login holds the single definition of the stored form of a university
// login. The tutor resolver's lookup, the per-phase unique index services build
// on it and the token login the scoping middleware resolves against all have to
// agree, so none of them may keep its own copy.
package login

import "strings"

// Normalize puts a university login into the form tutor rows are stored in.
func Normalize(universityLogin string) string {
	return strings.TrimSpace(strings.ToLower(universityLogin))
}
