// Package tutorscope owns tutor team access control for every phase service that
// models teams.
//
// A tutor is a course editor recorded as responsible for exactly one team of a
// course phase. Two decisions have to be made identically by every such service,
// or the guarantee is only as strong as its weakest implementation:
//
//   - Reads. Middleware resolves the tutor's team and stores it on the request so
//     handlers can filter what they return. It fails OPEN: an editor whose tutor
//     record cannot be resolved (no university login on the token, no tutor row)
//     keeps full read access.
//   - Writes. AuthorizeWrite resolves the same request into a write scope and
//     fails CLOSED: that same unresolvable editor is denied.
//
// The asymmetry is deliberate. Reusing the read gate for writes would hand every
// editor the resolver cannot place unrestricted write access to every team of the
// phase, which is the opposite of what tutor scoping is for.
//
// A service supplies the tutor lookup by implementing Resolver, or by using
// NewPgxResolver against a tutor table with the canonical shape documented on
// that constructor.
package tutorscope

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prompt-edu/prompt-sdk/keycloakTokenVerifier"
)

// Resolver resolves which team a tutor is assigned to. It is transport-agnostic
// so the lookup stays a one-method adapter over the service's own queries.
type Resolver = keycloakTokenVerifier.TutorTeamResolver

// TeamIDKey is the gin context key under which the resolved tutor team is stored.
const TeamIDKey = keycloakTokenVerifier.TutorTeamIDKey

// Middleware resolves the requesting tutor's team and stores it on the request.
// Lecturers, admins and editors with no resolvable tutor record pass through
// untouched, so reads stay unrestricted for them. A resolver failure other than
// "no such tutor" aborts with 500 rather than silently widening access.
//
// Install it on every route that reads or writes team-scoped data, including the
// write routes: AuthorizeWrite reports a misconfiguration if it is missing.
func Middleware(resolver Resolver) gin.HandlerFunc {
	return keycloakTokenVerifier.TutorScopingMiddleware(resolver)
}

// TeamID returns the tutor's scoped team and whether read scoping applies to this
// request. It returns (uuid.Nil, false) when the caller is unscoped, which for
// reads means unrestricted. Use AuthorizeWrite for write decisions: an unscoped
// caller is not necessarily allowed to write.
func TeamID(c *gin.Context) (uuid.UUID, bool) {
	return keycloakTokenVerifier.GetTutorTeamID(c)
}

// NormalizeLogin puts a university login into the form tutor rows are stored in.
// Services must apply it when writing tutor records, so the resolver's lookup and
// the unique index on (course_phase_id, university_login) agree with the token
// login the middleware resolves against.
func NormalizeLogin(login string) string {
	return strings.TrimSpace(strings.ToLower(login))
}
