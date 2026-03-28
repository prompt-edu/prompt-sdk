package promptSDK

import (
	"github.com/gin-gonic/gin"
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
