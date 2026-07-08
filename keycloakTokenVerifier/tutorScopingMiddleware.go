package keycloakTokenVerifier

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// TutorTeamIDKey is the gin context key under which the resolved tutor team ID is stored.
const TutorTeamIDKey = "tutorTeamID"

// TutorTeamResolver is implemented by each service against its own database. It
// is transport-agnostic (no gin types) so the lookup stays a one-method adapter
// over the service's sqlc queries.
type TutorTeamResolver interface {
	ResolveTutorTeam(ctx context.Context, coursePhaseID uuid.UUID, universityLogin string) (uuid.UUID, error)
}

// TutorScopingMiddleware resolves the requesting tutor's assigned team and stores
// it in the gin context for handlers to scope their responses. Only CourseEditor
// users registered as a tutor are scoped; lecturers, admins and unregistered
// editors pass through untouched. pgx.ErrNoRows from the resolver means "not a
// tutor" (full editor access); any other resolver error fails closed with 500.
func TutorScopingMiddleware(resolver TutorTeamResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenUser, ok := GetTokenUser(c)
		if !ok || !tokenUser.IsEditor || tokenUser.IsLecturer {
			c.Next()
			return
		}

		login := strings.TrimSpace(strings.ToLower(tokenUser.UniversityLogin))
		if login == "" {
			c.Next()
			return
		}

		coursePhaseID, err := uuid.Parse(c.Param("coursePhaseID"))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid course phase id"})
			return
		}

		teamID, err := resolver.ResolveTutorTeam(c.Request.Context(), coursePhaseID, login)
		if errors.Is(err, pgx.ErrNoRows) {
			c.Next()
			return
		}
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "access check failed"})
			return
		}

		c.Set(TutorTeamIDKey, teamID)
		c.Next()
	}
}

// GetTutorTeamID returns the tutor's scoped team and whether scoping applies to
// this request. It returns (uuid.Nil, false) when no scoping is active.
func GetTutorTeamID(c *gin.Context) (uuid.UUID, bool) {
	if v, exists := c.Get(TutorTeamIDKey); exists {
		if id, ok := v.(uuid.UUID); ok {
			return id, true
		}
	}
	return uuid.Nil, false
}
