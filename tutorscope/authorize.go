package tutorscope

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/prompt-edu/prompt-sdk/keycloakTokenVerifier"
)

var (
	// ErrNotAuthenticated means no token user reached the handler. Render it as 401.
	ErrNotAuthenticated = errors.New("no authenticated user on the request")

	// ErrWriteDenied means the caller may not write this team. Render it as 403.
	ErrWriteDenied = errors.New("access restricted to assigned team")

	// ErrScopingNotApplied means the route admits editors but is missing Middleware,
	// so no tutor record could have been resolved. It is a wiring bug, not a denial:
	// render it as 500 rather than telling a legitimate tutor they have no access.
	ErrScopingNotApplied = errors.New("tutor scoping middleware is not installed on this route")
)

// Access is the write scope resolved for one request.
//
// Authorization is decided from the team resolved at the start of the request, so a
// tutor whose assignment changes mid-request finishes with the one they started
// with. This matches the read-side semantics.
type Access struct {
	// TeamID is the only team the caller may write. Meaningful only when Confined.
	TeamID uuid.UUID

	// Confined reports whether the caller is restricted to TeamID.
	Confined bool
}

// AuthorizeWrite resolves the caller's write scope, failing closed:
//
//   - PROMPT_Admin and course lecturers write any team of the phase.
//   - A course editor with a resolved tutor team is confined to that team.
//   - Everyone else is denied, including an editor whose tutor record could not be
//     resolved. Reads let that editor through; writes must not.
//
// The global PROMPT_Lecturer role is deliberately not privileged here. It marks
// someone who may create courses, not a lecturer of this course, so honoring the
// raw role would unscope a tutor who happens to hold it. Such a user is admitted
// as a course editor and stays confined to their team, consistent with their reads.
//
// Middleware must be installed on the route for editors to be resolvable; if it is
// not, an editor yields ErrScopingNotApplied instead of a silent denial.
func AuthorizeWrite(c *gin.Context) (Access, error) {
	tokenUser, ok := keycloakTokenVerifier.GetTokenUser(c)
	if !ok {
		return Access{}, ErrNotAuthenticated
	}

	if tokenUser.Roles[keycloakTokenVerifier.PromptAdmin] || tokenUser.IsLecturer {
		return Access{}, nil
	}

	if !tokenUser.IsEditor {
		return Access{}, ErrWriteDenied
	}

	if !keycloakTokenVerifier.TutorScopingApplied(c) {
		return Access{}, ErrScopingNotApplied
	}

	teamID, scoped := keycloakTokenVerifier.GetTutorTeamID(c)
	if !scoped {
		return Access{}, ErrWriteDenied
	}

	return Access{TeamID: teamID, Confined: true}, nil
}

// Unrestricted reports whether the caller may write any team of the phase.
func (a Access) Unrestricted() bool {
	return !a.Confined
}

// AllowsTeam reports whether a write targeting teamID is in scope. Check it against
// the team a write moves a participant *into*, before touching the database. The
// team the participant is moved *out of* is enforced by Guard, which the database
// applies to the row as it exists at write time.
func (a Access) AllowsTeam(teamID uuid.UUID) bool {
	return !a.Confined || a.TeamID == teamID
}

// Guard returns the source-team guard to pass to a mutating statement: NULL when
// the caller is unrestricted, the tutor's team when confined.
//
// Fold it into the statement rather than reading the current team first and then
// writing, so authorization and mutation are atomic and no concurrent edit can move
// the row in between. With sqlc, take it as an optional parameter and compare
// against the stored team:
//
//	-- name: UpdateTeam :execrows
//	UPDATE allocations SET team_id = @team_id
//	WHERE course_participation_id = @course_participation_id
//	  AND course_phase_id = @course_phase_id
//	  AND team_id = COALESCE(sqlc.narg('expected_team_id')::uuid, team_id);
//
// Zero affected rows then means the guard did not match, which is ErrWriteDenied
// for a confined caller. An INSERT ... ON CONFLICT DO UPDATE carries the same
// guard in its WHERE clause, which lets a tutor pick up an unallocated participant
// while still refusing one who sits in another team.
func (a Access) Guard() pgtype.UUID {
	if !a.Confined {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: a.TeamID, Valid: true}
}
