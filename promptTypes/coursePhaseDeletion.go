package promptTypes

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prompt-edu/prompt-sdk/keycloakTokenVerifier"
	"github.com/sirupsen/logrus"
)

// CoursePhaseDeletionRequest represents the payload used to trigger the deletion of a course phase.
// It is sent to a module the moment a course phase is permanently deleted, giving the module a
// chance to remove all data it stores for that phase.
type CoursePhaseDeletionRequest struct {
	// CoursePhaseID is the unique identifier of the course phase being deleted.
	// The module should delete everything it stores that is scoped to this phase.
	CoursePhaseID uuid.UUID `json:"coursePhaseID"`
}

// CoursePhaseDeletionHandler defines the interface that modules must implement to support
// course phase deletion. Any module that stores course phase-specific data should implement
// this interface to ensure their data is properly removed when a course phase is deleted.
type CoursePhaseDeletionHandler interface {
	// HandleCoursePhaseDeletion processes the deletion of module-specific data scoped to the
	// course phase in the request. Implementations should:
	//   - Delete all data, settings, and state stored for the given course phase
	//   - Be idempotent: deleting a phase for which no data exists is a success, and
	//     re-running the deletion must not fail
	//
	// Returns an error if the deletion operation fails for any reason.
	HandleCoursePhaseDeletion(c *gin.Context, req CoursePhaseDeletionRequest) error
}

// RegisterCoursePhaseDeletionEndpoint registers the standardized POST /delete endpoint for course
// phase deletion. Because permanently removing a phase's data is a destructive, course-spanning
// operation, the SDK protects the endpoint with a PromptAdmin-only middleware itself, rather than
// accepting an authorization middleware from the caller. This mirrors the privacy data deletion
// endpoint.
//
// The endpoint standardizes the response codes:
//   - 200 OK: The deletion of data for the course phase was successful. This is also returned if
//     there was no data stored for that phase.
//   - 400 Bad Request: The request did not match the expected format.
//   - 500 Internal Server Error: Something went wrong and the deletion may not have been successful.
//
// Because the handler is expected to be idempotent, the endpoint returns 200 OK even when the
// module stored no data for the phase.
//
// Internal errors are not exposed to the caller, but are logged.
//
// Example endpoint path: POST /self-team-allocation/api/course_phase/:coursePhaseID/delete
func RegisterCoursePhaseDeletionEndpoint(router *gin.RouterGroup, handler CoursePhaseDeletionHandler) {

	adminOnlyMiddleware := keycloakTokenVerifier.AuthenticationMiddleware(keycloakTokenVerifier.PromptAdmin)
	router.POST("/delete", adminOnlyMiddleware, func(c *gin.Context) {
		var req CoursePhaseDeletionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			logrus.Error("caller passed invalid request body")
			c.JSON(http.StatusBadRequest, gin.H{"error": "request body for deletion does not match CoursePhaseDeletionRequest as defined in the SDK"})
			return
		}

		if err := handler.HandleCoursePhaseDeletion(c, req); err != nil {
			logrus.Error("course phase deletion was not successful, error: ", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process course phase deletion"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": "Course phase deletion request executed"})
	})
}
