// Package tutorscope owns tutor team access control for every phase service that
// models teams.
//
// A tutor is a course editor recorded as responsible for one or more teams of a
// course phase. Two decisions have to be made identically by every such service,
// or the guarantee is only as strong as its weakest implementation:
//
//   - Reads. Middleware (one team per tutor) or TeamsMiddleware (one or more) resolves
//     the tutor's teams and stores them on the request so handlers can filter what
//     they return. Once the course phase has tutors it
//     fails CLOSED: an editor who is none of them, including one whose token
//     carries no university login, is denied with 403. In a phase without tutors
//     editors keep full read access.
//   - Writes. AuthorizeWrite resolves the same request into a write scope and
//     fails CLOSED: an editor without a resolved tutor team is denied, and so is a
//     tutor of several teams, since a write scope covers exactly one team.
//
// In a phase without tutors the two differ on purpose: editors read every team but
// write none. Reusing the read gate for writes would hand them unrestricted write
// access to every team of the phase, which is the opposite of what tutor scoping
// is for.
//
// A service supplies the tutor lookup by implementing Resolver or TeamsResolver, or
// by using NewPgxResolver or NewPgxTeamsResolver against a tutor table with the
// canonical shape documented on those constructors.
package tutorscope

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prompt-edu/prompt-sdk/internal/login"
	"github.com/prompt-edu/prompt-sdk/keycloakTokenVerifier"
)

// Resolver resolves which team a tutor is assigned to. It is transport-agnostic
// so the lookup stays a one-method adapter over the service's own queries.
type Resolver = keycloakTokenVerifier.TutorTeamResolver

// TeamsResolver resolves every team a tutor is assigned to. It follows the error
// contract of Resolver; an empty list with a nil error fails closed like
// ErrNotATutor.
type TeamsResolver = keycloakTokenVerifier.TutorTeamsResolver

// ErrNotATutor is what a Resolver returns when the course phase has tutors and the
// login belongs to none of them. Middleware answers it with 403. A Resolver that
// returns pgx.ErrNoRows instead says the phase has no tutors, which leaves the
// editor unscoped.
var ErrNotATutor = keycloakTokenVerifier.ErrNotATutor

// TeamIDKey is the gin context key under which the first resolved tutor team is stored.
const TeamIDKey = keycloakTokenVerifier.TutorTeamIDKey

// TeamIDsKey is the gin context key under which every resolved tutor team is stored.
const TeamIDsKey = keycloakTokenVerifier.TutorTeamIDsKey

// Middleware resolves the requesting tutor's team and stores it on the request.
// Lecturers and admins pass through untouched. An editor is scoped to their team
// when they are a tutor, denied with 403 when the phase has tutors and they are
// none of them, and passed through unscoped in a phase without tutors. Any other
// resolver failure aborts with 500 rather than silently widening access.
//
// Install it on every route that reads or writes team-scoped data, including the
// write routes: AuthorizeWrite reports a misconfiguration if it is missing.
func Middleware(resolver Resolver) gin.HandlerFunc {
	return keycloakTokenVerifier.TutorScopingMiddleware(resolver)
}

// TeamsMiddleware is Middleware for tutors who can be assigned to several teams: it
// scopes an editor to every team the resolver returns. The read rules are the same,
// and an empty team list is denied with 403 like an editor who is no tutor.
func TeamsMiddleware(resolver TeamsResolver) gin.HandlerFunc {
	return keycloakTokenVerifier.TutorTeamsScopingMiddleware(resolver)
}

// TeamID returns the tutor's scoped team and whether read scoping applies to this
// request. It returns (uuid.Nil, false) when the caller is unscoped, which for
// reads means unrestricted. Use AuthorizeWrite for write decisions: an unscoped
// caller is not necessarily allowed to write.
//
// Deprecated: a tutor can be assigned to several teams. Use TeamIDs. For such a
// tutor TeamID reports only the first team, so a handler filtering by it shows
// less, never more.
func TeamID(c *gin.Context) (uuid.UUID, bool) {
	return keycloakTokenVerifier.GetTutorTeamID(c)
}

// TeamIDs returns every team the tutor is scoped to and whether read scoping
// applies to this request. It returns (nil, false) when the caller is unscoped,
// which for reads means unrestricted; when it reports true the list is never empty.
// Use AuthorizeWrite for write decisions.
func TeamIDs(c *gin.Context) ([]uuid.UUID, bool) {
	return keycloakTokenVerifier.GetTutorTeamIDs(c)
}

// NormalizeLogin puts a university login into the form tutor rows are stored in.
// Services must apply it when writing tutor records, so the resolver's lookup and
// the unique index on (course_phase_id, university_login) agree with the token
// login the middleware resolves against. Store NULL rather than the empty string
// when it returns "": see NewPgxResolver.
func NormalizeLogin(universityLogin string) string {
	return login.Normalize(universityLogin)
}
