package testutils

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prompt-edu/prompt-sdk/keycloakTokenVerifier"
	"github.com/sirupsen/logrus"
)

func MockAuthMiddlewareWithEmail(mockRoles []string, email, matriculationNumber, universityLogin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRoles := make(map[string]bool)
		for _, role := range mockRoles {
			userRoles[role] = true
		}
		c.Set("userRoles", userRoles)
		c.Set("userEmail", email)
		c.Set("matriculationNumber", matriculationNumber)
		c.Set("universityLogin", universityLogin)
		c.Set("firstName", "John")
		c.Set("lastName", "Doe")

		keycloakTokenVerifier.SetTokenUser(c, keycloakTokenVerifier.TokenUser{
			Roles:           userRoles,
			Email:           email,
			UniversityLogin: universityLogin,
		})

		logrus.Info("MockAuthMiddleware: Mocked user mail: ", email)
		c.Next()
	}
}

func MockAuthMiddleware(mockRoles []string) gin.HandlerFunc {
	return MockAuthMiddlewareWithEmail(mockRoles, "", "", "")
}

func DefaultMockAuthMiddleware() gin.HandlerFunc {
	return MockAuthMiddlewareWithParticipation(nil, uuid.MustParse("33333333-3333-3333-3333-333333333333"))
}

func MockAuthMiddlewareWithParticipation(mockRoles []string, courseParticipationID uuid.UUID) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRoles := make(map[string]bool)
		for _, role := range mockRoles {
			userRoles[role] = true
		}

		c.Set("userRoles", userRoles)
		c.Set("userEmail", "test@example.com")
		c.Set("matriculationNumber", "0000000")
		c.Set("universityLogin", "testuser")
		c.Set("firstName", "Test")
		c.Set("lastName", "User")
		c.Set("courseParticipationID", courseParticipationID)

		keycloakTokenVerifier.SetTokenUser(c, keycloakTokenVerifier.TokenUser{
			Roles:                 userRoles,
			Email:                 "test@example.com",
			UniversityLogin:       "testuser",
			CourseParticipationID: courseParticipationID,
		})

		logrus.Info("MockAuthMiddleware: set courseParticipationID ", courseParticipationID)
		c.Next()
	}
}

// MockTutorEditorMiddleware creates a mock for a CourseEditor who is registered as a tutor.
// The universityLogin is used by tutorScopingMiddleware to look up the tutor's team.
func MockTutorEditorMiddleware(universityLogin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRoles := map[string]bool{keycloakTokenVerifier.CourseEditor: true}
		c.Set("userRoles", userRoles)
		c.Set("userEmail", universityLogin+"@tum.de")
		c.Set("universityLogin", universityLogin)

		keycloakTokenVerifier.SetTokenUser(c, keycloakTokenVerifier.TokenUser{
			Roles:           userRoles,
			Email:           universityLogin + "@tum.de",
			UniversityLogin: universityLogin,
			IsEditor:        true,
		})

		c.Next()
	}
}
