package promptTypes

import (
	"github.com/gin-gonic/gin"
	"github.com/prompt-edu/prompt-sdk/keycloakTokenVerifier"
)

// RegisterConfigModule wires a phase config module onto the given router group: it builds the
// role-based auth middleware from the allowed roles and registers the standardized /config
// endpoint behind it. Replaces the per-service setupConfigRouter glue.
func RegisterConfigModule(group *gin.RouterGroup, handler PhaseConfigHandler, roles ...string) {
	RegisterConfigEndpoint(group, keycloakTokenVerifier.AuthenticationMiddleware(roles...), handler)
}

// RegisterCopyModule wires a phase copy module onto the given router group: it builds the
// role-based auth middleware from the allowed roles and registers the standardized /copy
// endpoint behind it. Replaces the per-service setupCopyRouter glue.
func RegisterCopyModule(group *gin.RouterGroup, handler PhaseCopyHandler, roles ...string) {
	RegisterCopyEndpoint(group, keycloakTokenVerifier.AuthenticationMiddleware(roles...), handler)
}

// RegisterPhaseDeletionModule wires a phase deletion module onto the given router group: it
// registers the standardized /delete endpoint. The endpoint protects itself (PromptAdmin and
// CourseLecturer, mirroring the core server), so no roles are taken here. It is named after the
// phase.deletion capability to keep it apart from the privacy deletion handled by
// RegisterPrivacyModule.
//
// The group's path must contain ":coursePhaseID" (e.g. ".../api/course_phase/:coursePhaseID"),
// as required by RegisterCoursePhaseDeletionEndpoint.
func RegisterPhaseDeletionModule(group *gin.RouterGroup, handler CoursePhaseDeletionHandler) {
	RegisterCoursePhaseDeletionEndpoint(group, handler)
}

// RegisterPrivacyModule wires a phase privacy module onto the given router group by registering the
// standardized privacy data-export endpoint and, when a deletion handler is supplied, the
// data-deletion endpoint. Both endpoints manage their own authentication (export accepts any valid
// token, deletion is admin-only), so no roles are taken here. Pass a nil deletion handler to
// register the export endpoint only.
func RegisterPrivacyModule(
	group *gin.RouterGroup,
	export PrivacyDataExportHandler,
	deletion PrivacyDataDeletionHandler,
	allowedUploadHosts []string,
) {
	RegisterPrivacyDataExportEndpoint(group, export, allowedUploadHosts)
	if deletion != nil {
		RegisterPrivacyDataDeletionEndpoint(group, deletion)
	}
}
