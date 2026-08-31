package promptTypes

// Privacy route constants used when registering endpoints via RegisterPrivacyDataExportEndpoint
// and RegisterPrivacyDataDeletionEndpoint.
const (
	// PrivacyRouteDataExport is the POST endpoint path for triggering a privacy data export.
	PrivacyRouteDataExport string = "/privacy/data-export"

	// PrivacyRouteDataDeletion is the POST endpoint path for triggering a privacy data deletion.
	PrivacyRouteDataDeletion string = "/privacy/data-deletion"
)
