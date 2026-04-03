package keycloakTokenVerifier

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prompt-edu/prompt-sdk/keycloakTokenVerifier/keycloakCoreRequests"
	"github.com/sirupsen/logrus"
)

// SubjectIdentifiers is re-exported from keycloakCoreRequests for use by SDK consumers.
type SubjectIdentifiers = keycloakCoreRequests.SubjectIdentifiers

func SubjectIdentifierMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing Authorization Header",
			})
			return
		}
		subjIdent, err := keycloakCoreRequests.GetSubjectIdentifiers(KeycloakTokenVerifierSingleton.CoreURL, auth)
		if err != nil {
			logrus.Error("get subject identifiers Middleware errored ", err)
			if errors.Is(err, keycloakCoreRequests.ErrUnauthorized) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			} else {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve subject identifiers"})
			}
			return
		}
		c.Set("subjectIdentifiers", subjIdent)
		c.Next()
	}
}
