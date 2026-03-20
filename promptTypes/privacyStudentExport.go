package promptTypes

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prompt-edu/prompt-sdk/utils"
)

// PrivacyDataExportRequest is the payload the core server sends to each microservice
// to trigger a privacy data export.
type PrivacyDataExportRequest struct {
	// Subject contains all IDs needed to scope the export to one subject.
	Subject SubjectIdentifiers `json:"subject"`

	// PreSignedURL is an S3 presigned PUT URL the microservice must upload the zip to.
	// The object key (file name) and expiry are already encoded in this URL by the core.
	PreSignedURL string `json:"preSignedURL" binding:"required,url"`
}

// PrivacyDataExportHandler defines the interface that microservices must implement to
// support GDPR-compliant data exports. The implementation receives a pre-initialized
// Export and simply adds items to it — the SDK handles ZIP creation, upload, and cleanup.
//
// Example implementation:
//
//	func (h *myHandler) HandlePrivacyExportData(c *gin.Context, exp *utils.Export, subject SubjectIdentifiers) error {
//	    exp.AddJSON("User record", "user-record.json", func() (any, error) {
//	        return user.GetUserByID(c, subject.UserID)
//	    })
//	    return nil
//	}
type PrivacyDataExportHandler interface {
	// HandlePrivacyExportData adds the subject's data to the provided export.
	// The SDK creates the export, calls this method, then uploads and cleans up.
	HandlePrivacyExportData(c *gin.Context, exp *utils.Export, subject SubjectIdentifiers) error
}

// RegisterPrivacyDataExportEndpoint registers the standardized POST endpoint for privacy data exports.
// The core server calls this endpoint on each microservice when a privacy data export is requested.
//
// The endpoint handles the full export lifecycle:
//   - JSON request parsing and validation
//   - Authentication through the provided middleware
//   - Creating the export archive
//   - Calling the handler to populate it
//   - Uploading the archive to the presigned S3 URL
//   - Cleaning up temporary files
//
// Parameters:
//   - router: The Gin router group where the endpoint will be registered
//   - authMiddleware: Authentication middleware to protect the endpoint
//   - handler: Implementation of PrivacyDataExportHandler that populates the export
func RegisterPrivacyDataExportEndpoint(router *gin.RouterGroup, authMiddleware gin.HandlerFunc, handler PrivacyDataExportHandler) {
	router.POST(PrivacyRouteDataExport, authMiddleware, func(c *gin.Context) {
		var req PrivacyDataExportRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		exp, err := utils.NewExport()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer exp.Close()

		if err := handler.HandlePrivacyExportData(c, exp, req.Subject); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if err := exp.UploadTo(c.Request.Context(), req.PreSignedURL); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": "Privacy data export completed"})
	})
}
