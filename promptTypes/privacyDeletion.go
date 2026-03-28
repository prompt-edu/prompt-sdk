package promptTypes

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prompt-edu/prompt-sdk/keycloakTokenVerifier"
)

// PrivacyDataDeletionHandler defines the interface that microservices must implement to
// support GDPR-compliant data deletion. The implementation is responsible for
// permanently removing all data associated with the identified subject from the
// microservice's own storage. Implementations should treat the case where no data
// exists for the subject as a success (idempotent behavior).
type PrivacyDataDeletionHandler interface {
	// HandlePrivacyDeleteData permanently deletes all data belonging to the identified subject.
	// Returns an error if the deletion could not be completed.
	HandlePrivacyDeleteData(c *gin.Context, req keycloakTokenVerifier.SubjectIdentifiers) error
}

// RegisterPrivacyDataDeletionEndpoint registers the standardized POST endpoint for privacy data deletion.
// The core server calls this endpoint on each microservice when a privacy data deletion is requested.
//
// The endpoint handles:
//   - JSON request parsing and validation
//   - Authentication through the provided middleware
//   - Error handling and standardized responses
//   - Success confirmation messages
//
// Example endpoint path: POST /my-service/api/privacy/data-deletion
//
// Parameters:
//   - router: The Gin router group where the endpoint will be registered
//   - authMiddleware: Authentication middleware to protect the endpoint
//   - handler: Implementation of PrivacyDataDeletionHandler that performs the actual deletion
func RegisterPrivacyDataDeletionEndpoint(router *gin.RouterGroup, authMiddleware gin.HandlerFunc, handler PrivacyDataDeletionHandler) {

	subjectIdentifierMiddleware := keycloakTokenVerifier.SubjectIdentifierMiddleware()

	router.POST(PrivacyRouteDataDeletion, authMiddleware, subjectIdentifierMiddleware, func(c *gin.Context) {

		subjectIdentifiersVal, exists := c.Get("subjectIdentifiers")
		subjectIdentifiers, ok := subjectIdentifiersVal.(keycloakTokenVerifier.SubjectIdentifiers)
		if !exists || !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or no Authorization Header"})
			return
		}

		if err := handler.HandlePrivacyDeleteData(c, subjectIdentifiers); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process deletion"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": "Privacy data deletion request executed"})
	})
}
