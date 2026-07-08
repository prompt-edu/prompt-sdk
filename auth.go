package promptSDK

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prompt-edu/prompt-sdk/keycloakTokenVerifier"
)

// Re-exporting role constants from keycloakTokenVerifier
const (
	PromptAdmin    = keycloakTokenVerifier.PromptAdmin
	PromptLecturer = keycloakTokenVerifier.PromptLecturer
	CourseLecturer = keycloakTokenVerifier.CourseLecturer
	CourseEditor   = keycloakTokenVerifier.CourseEditor
	CourseStudent  = keycloakTokenVerifier.CourseStudent
)

func InitAuthenticationMiddleware(KeycloakURL, Realm, CoreURL string) error {
	return keycloakTokenVerifier.InitKeycloakTokenVerifier(KeycloakURL, Realm, CoreURL)
}

func AuthenticationMiddleware(allowedRoles ...string) gin.HandlerFunc {
	return keycloakTokenVerifier.AuthenticationMiddleware(allowedRoles...)
}

func SubjectIdentifierMiddleware() gin.HandlerFunc {
	return keycloakTokenVerifier.SubjectIdentifierMiddleware()
}

// TutorTeamResolver is implemented by each service against its own database to
// resolve which team a tutor (CourseEditor) is assigned to.
type TutorTeamResolver = keycloakTokenVerifier.TutorTeamResolver

// TutorTeamIDKey is the gin context key under which the resolved tutor team ID is stored.
const TutorTeamIDKey = keycloakTokenVerifier.TutorTeamIDKey

func TutorScopingMiddleware(resolver TutorTeamResolver) gin.HandlerFunc {
	return keycloakTokenVerifier.TutorScopingMiddleware(resolver)
}

func GetTutorTeamID(c *gin.Context) (uuid.UUID, bool) {
	return keycloakTokenVerifier.GetTutorTeamID(c)
}
