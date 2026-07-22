package promptTypes

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prompt-edu/prompt-sdk/keycloakTokenVerifier"
	"github.com/prompt-edu/prompt-sdk/utils"
	"github.com/stretchr/testify/require"
)

type stubConfigHandler struct{}

func (stubConfigHandler) HandlePhaseConfig(*gin.Context) (map[string]bool, error) {
	return map[string]bool{}, nil
}

type stubCopyHandler struct{}

func (stubCopyHandler) HandlePhaseCopy(*gin.Context, PhaseCopyRequest) error { return nil }

func registeredRoutes(t *testing.T, register func(*gin.RouterGroup)) map[string]bool {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	register(r.Group("/course_phase/:coursePhaseID"))

	routes := map[string]bool{}
	for _, route := range r.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	return routes
}

func TestRegisterConfigAndCopyModule(t *testing.T) {
	routes := registeredRoutes(t, func(g *gin.RouterGroup) {
		RegisterConfigModule(g, stubConfigHandler{}, keycloakTokenVerifier.PromptAdmin)
		RegisterCopyModule(g, stubCopyHandler{}, keycloakTokenVerifier.PromptAdmin)
	})

	require.True(t, routes["GET /course_phase/:coursePhaseID/config"])
	require.True(t, routes["POST /course_phase/:coursePhaseID/copy"])
}

func TestRegisterPrivacyModuleWithDeletion(t *testing.T) {
	export := func(*gin.Context, *utils.Export, keycloakTokenVerifier.SubjectIdentifiers) error { return nil }
	deletion := func(*gin.Context, keycloakTokenVerifier.SubjectIdentifiers) error { return nil }

	routes := registeredRoutes(t, func(g *gin.RouterGroup) {
		RegisterPrivacyModule(g, export, deletion, nil)
	})

	require.True(t, routes["POST /course_phase/:coursePhaseID"+PrivacyRouteDataExport])
	require.True(t, routes["POST /course_phase/:coursePhaseID"+PrivacyRouteDataDeletion])
}

func TestRegisterPrivacyModuleExportOnly(t *testing.T) {
	export := func(*gin.Context, *utils.Export, keycloakTokenVerifier.SubjectIdentifiers) error { return nil }

	routes := registeredRoutes(t, func(g *gin.RouterGroup) {
		RegisterPrivacyModule(g, export, nil, nil)
	})

	require.True(t, routes["POST /course_phase/:coursePhaseID"+PrivacyRouteDataExport])
	require.False(t, routes["POST /course_phase/:coursePhaseID"+PrivacyRouteDataDeletion])
}
