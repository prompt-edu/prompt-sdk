package keycloakTokenVerifier

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/prompt-edu/prompt-sdk/internal/login"
)

// TutorTeamIDKey is the gin context key under which the first resolved tutor team ID
// is stored. Readers that only know one team stay narrow: they see the first team of
// a tutor assigned to several, never more. Use TutorTeamIDsKey for the full list.
const TutorTeamIDKey = "tutorTeamID"

// TutorTeamIDsKey is the gin context key under which every resolved tutor team ID is
// stored, as a non-empty []uuid.UUID.
const TutorTeamIDsKey = "tutorTeamIDs"

// TutorScopingAppliedKey is the gin context key marking that TutorScopingMiddleware
// ran on this request. Write authorization needs to tell "this editor is not a tutor"
// apart from "the route forgot the middleware": both leave no team in the context, but
// only the first is a legitimate denial.
const TutorScopingAppliedKey = "tutorScopingApplied"

// ErrNotATutor is returned by a TutorTeamResolver or TutorTeamsResolver when the
// course phase has tutors and the login belongs to none of them.
var ErrNotATutor = errors.New("not a tutor of a course phase that has tutors")

// TutorTeamResolver is implemented by each service against its own database. It
// is transport-agnostic (no gin types) so the lookup stays a one-method adapter
// over the service's sqlc queries. The login is normalized and empty when the
// token carries none; a resolver must never match an empty login to a row.
//
// It resolves exactly one team per tutor. Services whose tutors can be assigned to
// several teams implement TutorTeamsResolver instead.
type TutorTeamResolver interface {
	ResolveTutorTeam(ctx context.Context, coursePhaseID uuid.UUID, universityLogin string) (uuid.UUID, error)
}

// TutorTeamsResolver resolves every team a tutor is assigned to in a course phase.
// The error contract matches TutorTeamResolver: pgx.ErrNoRows means the phase has no
// tutors, ErrNotATutor means it has tutors and the login is none of them. An empty
// slice with a nil error is treated like ErrNotATutor, so a resolver that finds no
// team can never widen access.
type TutorTeamsResolver interface {
	ResolveTutorTeams(ctx context.Context, coursePhaseID uuid.UUID, universityLogin string) ([]uuid.UUID, error)
}

// singleTeamResolver adapts a TutorTeamResolver to TutorTeamsResolver.
type singleTeamResolver struct {
	resolver TutorTeamResolver
}

func (s singleTeamResolver) ResolveTutorTeams(ctx context.Context, coursePhaseID uuid.UUID, universityLogin string) ([]uuid.UUID, error) {
	teamID, err := s.resolver.ResolveTutorTeam(ctx, coursePhaseID, universityLogin)
	if err != nil {
		return nil, err
	}
	return []uuid.UUID{teamID}, nil
}

// TutorScopingMiddleware scopes a tutor to the single team a TutorTeamResolver
// resolves. It behaves exactly like TutorTeamsScopingMiddleware with a one-team
// list.
func TutorScopingMiddleware(resolver TutorTeamResolver) gin.HandlerFunc {
	if resolver == nil {
		panic("TutorScopingMiddleware: resolver must not be nil")
	}
	return TutorTeamsScopingMiddleware(singleTeamResolver{resolver: resolver})
}

// TutorTeamsScopingMiddleware resolves every team the requesting tutor is assigned
// to and stores them in the gin context for handlers to scope their responses. Only
// CourseEditor users are checked; lecturers and admins pass through untouched. A
// resolved team list scopes the editor to those teams, ErrNotATutor or an empty
// list aborts with 403, and pgx.ErrNoRows means the phase has no tutors, so the
// editor passes through unscoped. Any other resolver error fails closed with 500.
func TutorTeamsScopingMiddleware(resolver TutorTeamsResolver) gin.HandlerFunc {
	if resolver == nil {
		panic("TutorTeamsScopingMiddleware: resolver must not be nil")
	}
	return func(c *gin.Context) {
		// Set before any branch, so the marker means "the middleware ran", not
		// "the middleware resolved something".
		c.Set(TutorScopingAppliedKey, true)

		tokenUser, ok := GetTokenUser(c)
		if !ok || !tokenUser.IsEditor || tokenUser.IsLecturer {
			c.Next()
			return
		}

		coursePhaseID, err := uuid.Parse(c.Param("coursePhaseID"))
		if err != nil || coursePhaseID == uuid.Nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid course phase id"})
			return
		}

		teamIDs, err := resolver.ResolveTutorTeams(c.Request.Context(), coursePhaseID, login.Normalize(tokenUser.UniversityLogin))
		if errors.Is(err, pgx.ErrNoRows) {
			c.Next()
			return
		}
		if errors.Is(err, ErrNotATutor) || (err == nil && len(teamIDs) == 0) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "access restricted to the tutors of this course phase"})
			return
		}
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "access check failed"})
			return
		}

		c.Set(TutorTeamIDsKey, append([]uuid.UUID(nil), teamIDs...))
		c.Set(TutorTeamIDKey, teamIDs[0])
		c.Next()
	}
}

// GetTutorTeamIDs returns every team the tutor is scoped to and whether scoping
// applies to this request. It returns (nil, false) when no scoping is active; when
// it reports true the list is never empty.
func GetTutorTeamIDs(c *gin.Context) ([]uuid.UUID, bool) {
	if v, exists := c.Get(TutorTeamIDsKey); exists {
		if ids, ok := v.([]uuid.UUID); ok && len(ids) > 0 {
			return append([]uuid.UUID(nil), ids...), true
		}
	}
	return nil, false
}

// GetTutorTeamID returns the tutor's first scoped team and whether scoping applies
// to this request. It returns (uuid.Nil, false) when no scoping is active.
//
// Deprecated: a tutor can be assigned to several teams. Use GetTutorTeamIDs. This
// function stays narrow on purpose: for a tutor of several teams it reports only the
// first, so a caller that filters by it shows less, never more.
func GetTutorTeamID(c *gin.Context) (uuid.UUID, bool) {
	if v, exists := c.Get(TutorTeamIDKey); exists {
		if id, ok := v.(uuid.UUID); ok {
			return id, true
		}
	}
	return uuid.Nil, false
}

// TutorScopingApplied reports whether TutorScopingMiddleware ran on this request.
func TutorScopingApplied(c *gin.Context) bool {
	applied, exists := c.Get(TutorScopingAppliedKey)
	if !exists {
		return false
	}
	ran, ok := applied.(bool)
	return ok && ran
}
