package promptTypes

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// StudentExportResult is returned by a microservice after it has successfully uploaded
// the student data export. The core server can use this to confirm completion and
// record when the export was produced.
type StudentExportResult struct {
	// Timestamp is the time at which the microservice completed the export.
	Timestamp time.Time `json:"timestamp"`

	// Data contains any additional microservice-specific metadata about the export.
	Data json.RawMessage `json:"data"`
}

// StudentExportRequest is the payload the core server sends to each microservice
// to trigger a student data export. The microservice must collect all data belonging
// to the identified student, package it into a zip file, and upload it via a simple
// HTTP PUT to the provided presigned URL. No AWS credentials are required on the
// microservice side — the presigned URL grants time-limited write access to a
// specific S3 object whose key is determined by the core server at generation time.
type StudentExportRequest struct {
	// SubjectIdentifiers contains all IDs needed to scope the export to one subject.
	SubjectIdentifiers SubjectIdentifiers `json:"requester"`

	// PreSignedURL is an S3 presigned PUT URL the microservice must upload the zip to.
	// The object key (file name) and expiry are already encoded in this URL by the core.
	PreSignedURL string `json:"preSignedURL"`
}

// StudentExportHandler defines the interface that microservices must implement to
// support GDPR-compliant student data exports. The implementation is responsible for
// collecting all student-related data, creating a zip archive, and uploading it to S3
// via the presigned URL provided in the request.
type StudentExportHandler interface {
	// HandleExportStudentData collects and uploads the student's data.
	// It receives the full export request including the presigned S3 upload URL.
	// Returns a StudentExportResult on success, or an error if the export failed.
	HandleExportStudentData(c *gin.Context, req StudentExportRequest) (StudentExportResult, error)
}

// RegisterStudentExportEndpoint registers the standardized POST endpoint for student data exports.
// The core server calls this endpoint on each microservice when a student data export is requested.
//
// The endpoint handles:
//   - JSON request parsing and validation
//   - Authentication through the provided middleware
//   - Error handling and standardized responses
//
// Example endpoint path: POST /my-service/api/privacy/student-data-export
//
// Parameters:
//   - router: The Gin router group where the endpoint will be registered
//   - authMiddleware: Authentication middleware to protect the endpoint
//   - handler: Implementation of StudentExportHandler that performs the actual export and upload
func RegisterStudentExportEndpoint(router *gin.RouterGroup, authMiddleware gin.HandlerFunc, handler StudentExportHandler) {
	router.POST(PrivacyRouteStudentDataExport, authMiddleware, func(c *gin.Context) {
		var req StudentExportRequest
		if errRead := c.ShouldBindJSON(&req); errRead != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": errRead.Error()})
			return
		}

		res, errExport := handler.HandleExportStudentData(c, req)
		if errExport != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
      return
		}

		c.JSON(http.StatusOK, res)
	})
}
