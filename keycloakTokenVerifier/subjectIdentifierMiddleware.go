package keycloakTokenVerifier

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prompt-edu/prompt-sdk/keycloakTokenVerifier/keycloakCoreRequests"
)

// SubjectIdentifiers is re-exported from keycloakCoreRequests for use by SDK consumers.
type SubjectIdentifiers = keycloakCoreRequests.SubjectIdentifiers

func SubjectIdentifierMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "missing Authorization Header",
			})
			return
		}
		subjIdent, err := keycloakCoreRequests.GetSubjectIdentifiers(KeycloakTokenVerifierSingleton.CoreURL, auth)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
		c.Set("subjectIdentifiers", subjIdent)
		c.Next()
	}
}
