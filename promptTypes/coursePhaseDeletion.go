package promptTypes

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prompt-edu/prompt-sdk/keycloakTokenVerifier"
	"github.com/sirupsen/logrus"
)

// CoursePhaseDeletionHandler defines the interface that modules must implement to support
// course phase deletion. Any module that stores course phase-specific data should implement
// this interface to ensure their data is properly removed when a course phase is deleted.
type CoursePhaseDeletionHandler interface {
	// HandleCoursePhaseDeletion deletes all module-specific data scoped to the given course
	// phase. It is called the moment a course phase is permanently deleted, giving the module
	// a chance to remove everything it stores for that phase. Implementations should:
	//   - Delete all data, settings, and state stored for the given course phase
	//   - Be idempotent: deleting a phase for which no data exists is a success, and
	//     re-running the deletion must not fail
	//
	// Returns an error if the deletion operation fails for any reason.
	HandleCoursePhaseDeletion(c *gin.Context, coursePhaseID uuid.UUID) error
}

// RegisterCoursePhaseDeletionEndpoint registers the standardized POST /delete endpoint for course
// phase deletion. Because permanently removing a phase's data is a destructive operation, the SDK
// protects the endpoint itself rather than accepting an authorization middleware from the caller:
// it allows PromptAdmin and CourseLecturer, mirroring the roles the core server requires to delete
// a course phase.
//
// The endpoint MUST be registered on a router group whose path contains ":coursePhaseID"
// (e.g. ".../api/course_phase/:coursePhaseID"). The course phase ID is read from that path
// parameter, and it is also required to resolve the CourseLecturer role.
//
// The endpoint standardizes the response codes:
//   - 200 OK: The deletion of data for the course phase was successful. This is also returned if
//     there was no data stored for that phase.
//   - 400 Bad Request: The course phase ID in the path was missing or malformed.
//   - 500 Internal Server Error: Something went wrong and the deletion may not have been successful.
//
// Because the handler is expected to be idempotent, the endpoint returns 200 OK even when the
// module stored no data for the phase.
//
// Internal errors are not exposed to the caller, but are logged.
//
// Example endpoint path: POST /self-team-allocation/api/course_phase/:coursePhaseID/delete
func RegisterCoursePhaseDeletionEndpoint(router *gin.RouterGroup, handler CoursePhaseDeletionHandler) {

	authMiddleware := keycloakTokenVerifier.AuthenticationMiddleware(keycloakTokenVerifier.PromptAdmin, keycloakTokenVerifier.CourseLecturer)
	router.POST("/delete", authMiddleware, func(c *gin.Context) {
		coursePhaseID, err := uuid.Parse(c.Param("coursePhaseID"))
		if err != nil {
			logrus.Error("caller passed invalid or missing coursePhaseID path parameter")
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or missing coursePhaseID"})
			return
		}

		if err := handler.HandleCoursePhaseDeletion(c, coursePhaseID); err != nil {
			logrus.Error("course phase deletion was not successful, error: ", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process course phase deletion"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": "Course phase deletion request executed"})
	})
}
