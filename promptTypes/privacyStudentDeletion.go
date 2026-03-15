package promptTypes

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// StudentDeletionHandler defines the interface that microservices must implement to
// support GDPR-compliant data deletion. The implementation is responsible for
// permanently removing all data associated with the identified subject from the
// microservice's own storage. Implementations should treat the case where no data
// exists for the subject as a success (idempotent behavior).
type StudentDeletionHandler interface {
	// HandleDeleteStudentData permanently deletes all data belonging to the identified subject.
	// Returns an error if the deletion could not be completed.
	HandleDeleteStudentData(c *gin.Context, req SubjectIdentifiers) error
}

// StudentDeletionRequest wraps the subject identifiers for deletion requests.
// This mirrors the structure used by other privacy-related endpoints, where the
// subject identifiers are nested under a top-level "subject" field.
type StudentDeletionRequest struct {
	Subject SubjectIdentifiers `json:"subject" binding:"required"`
}

// RegisterStudentDeletionEndpoint registers the standardized POST endpoint for student data deletion.
// The core server calls this endpoint on each microservice when a student data deletion is requested.
//
// The endpoint handles:
//   - JSON request parsing and validation
//   - Authentication through the provided middleware
//   - Error handling and standardized responses
//   - Success confirmation messages
//
// Example endpoint path: POST /my-service/api/privacy/student-data-deletion
//
// Parameters:
//   - router: The Gin router group where the endpoint will be registered
//   - authMiddleware: Authentication middleware to protect the endpoint
//   - handler: Implementation of StudentDeletionHandler that performs the actual deletion
func RegisterStudentDeletionEndpoint(router *gin.RouterGroup, authMiddleware gin.HandlerFunc, handler StudentDeletionHandler) {
	router.POST(PrivacyRouteStudentDataDeletion, authMiddleware, func(c *gin.Context) {
		var req StudentDeletionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := handler.HandleDeleteStudentData(c, req.Subject); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": "Student data deletion request executed"})
	})
}
