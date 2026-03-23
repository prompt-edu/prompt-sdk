package promptTypes

import (
	"net/http"
	"net/url"
	"strings"

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

// PrivacyDataExportHandler is called by the SDK to populate a privacy data export.
// The implementation receives a pre-initialized Export and simply adds items to it —
// the SDK handles ZIP creation, upload, and cleanup.
//
// Example:
//
//	func(c *gin.Context, exp *utils.Export, subject SubjectIdentifiers) error {
//	    exp.AddJSON("User record", "user-record.json", func() (any, error) {
//	        return db.GetUserByID(c, subject.UserID)
//	    })
//	    return nil
//	}
type PrivacyDataExportHandler func(c *gin.Context, exp *utils.Export, subject SubjectIdentifiers) error

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
//   - allowedUploadHosts: List of allowed hosts for the presigned upload URL.
//     If nil or empty, all hosts are allowed.
func RegisterPrivacyDataExportEndpoint(router *gin.RouterGroup, authMiddleware gin.HandlerFunc, handler PrivacyDataExportHandler, allowedUploadHosts []string) {
	router.POST(PrivacyRouteDataExport, authMiddleware, func(c *gin.Context) {
		var req PrivacyDataExportRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		parsed, err := url.Parse(req.PreSignedURL)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid upload URL"})
			return
		}

		host := parsed.Hostname()
		isLocal := host == "localhost" || host == "127.0.0.1"
		if parsed.Scheme != "https" && !isLocal {
			c.JSON(http.StatusBadRequest, gin.H{"error": "upload URL must use HTTPS"})
			return
		}

		if len(allowedUploadHosts) > 0 && !isAllowedHost(host, allowedUploadHosts) {
			c.JSON(http.StatusForbidden, gin.H{"error": "upload URL host not allowed"})
			return
		}

		exp, err := utils.NewExport()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create export"})
			return
		}
		defer exp.Close()

		if err := handler(c, exp, req.Subject); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process export"})
			return
		}

		if err := exp.UploadTo(c.Request.Context(), req.PreSignedURL); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upload export"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": "Privacy data export completed"})
	})
}

// isAllowedHost checks if the host matches any entry in the allowlist.
// Entries starting with "*." match any subdomain (e.g. "*.amazonaws.com" matches "s3.amazonaws.com").
func isAllowedHost(host string, allowed []string) bool {
	host = strings.ToLower(host)
	for _, a := range allowed {
		a = strings.ToLower(a)
		if strings.HasPrefix(a, "*.") {
			if strings.HasSuffix(host, a[1:]) {
				return true
			}
		} else if host == a {
			return true
		}
	}
	return false
}
