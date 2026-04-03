package promptTypes

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prompt-edu/prompt-sdk/keycloakTokenVerifier"
	"github.com/sirupsen/logrus"
)

// PrivacyDeletionRequest is the payload the core server sendsd to each microservice
// to trigger a privacy deletion request.
type PrivacyDeletionRequest struct {
	SubjectIdentifiers keycloakTokenVerifier.SubjectIdentifiers `json:"subjectIdentifiers"`
}

// A microservice should provide a PrivacyDataDeletionHandler. When a request for deletion
// comes in, the Endpoint registerd with (RegisterPrivacyDataDeletionEndpoint) will handle
// authentication and validation and then call the HandlePrivacyDeleteData of the Handler
// The HandlePrivacyDeleteData function is the main function which should delete all data
// related to the subject identified by the SubjectIdentifiers. Returning no error indicates
// success.
//
// Example:
//
//		func(c *gin.Context, subject keycloakTokenVerifier.SubjectIdentifiers) error {
//	     return someService.DeleteUser(c, subject.userID)
//		}
type PrivacyDataDeletionHandler interface {
	HandlePrivacyDeleteData(c *gin.Context, subject keycloakTokenVerifier.SubjectIdentifiers) error
}

// RegisterPrivacyDataDeletionEndpoint registers the standardized POST endpoint for privacy
// deletion requests. The core server calls this endpoint with an admin token on each microservice
// the moment a privacy deletion has been approved by an admin.
//
// The endpoint parses the request body and calls the handler. it also standardizes the response codes:
//   - 200 OK: The deletion of data for the subject was successful. This is also returned if there was no data stored about that subject
//   - 400 Bad Request: The request did not match the expected format. Specifically, the request body
//   - 500 InternalServerError: Something went wrong. And the deletion request may not have been successful
//
// Internal errors are not exposed to the caller and logged.
func RegisterPrivacyDataDeletionEndpoint(router *gin.RouterGroup, handler PrivacyDataDeletionHandler) {

	adminOnlyMiddleware := keycloakTokenVerifier.AuthenticationMiddleware(keycloakTokenVerifier.PromptAdmin)
	router.POST(PrivacyRouteDataDeletion, adminOnlyMiddleware, func(c *gin.Context) {

		var req PrivacyDeletionRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			logrus.Error("caller passed invalid request body")
			c.JSON(http.StatusBadRequest, gin.H{"error": "request body for deletion does not match PrivacyDeletionRequest as defined in the SDK"})
			return
		}

		if err := handler.HandlePrivacyDeleteData(c, req.SubjectIdentifiers); err != nil {
			logrus.Error("deletion was not successful for ", req.SubjectIdentifiers, ", error: ", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process deletion"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": "Privacy data deletion request executed"})
	})
}
