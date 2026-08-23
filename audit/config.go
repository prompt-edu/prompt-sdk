package audit

import "github.com/prompt-edu/prompt-sdk/utils"

// Enabled reports whether audit logging is switched on for this service. It is
// an administrator-controlled feature toggle: audit stays off unless
// AUDIT_ENABLED is explicitly set to "true", so wiring the middleware in is
// safe even before an admin turns it on.
func Enabled() bool {
	return utils.GetEnv("AUDIT_ENABLED", "") == "true"
}
