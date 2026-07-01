package keycloakTokenVerifier

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newStudentPathRouter wires the student-verification middleware exactly as
// AuthenticationMiddleware does, but without KeycloakMiddleware (which needs a
// live OIDC verifier). A pre-middleware seeds the TokenUser the way Keycloak
// would, and the terminal handler echoes the gate-relevant flags so the test
// can assert them. authMiddleware.go step 3 grants student access on
// TokenUser.IsStudentOfCourse, so that flag is what guards the CVE.
func newStudentPathRouter(t *testing.T, coreURL string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	u, err := url.Parse(coreURL)
	require.NoError(t, err)
	KeycloakTokenVerifierSingleton = &KeycloakTokenVerifier{CoreURL: *u}

	r := gin.New()
	grp := r.Group("/api/course_phase/:coursePhaseID")
	grp.GET("/resource",
		func(c *gin.Context) {
			SetTokenUser(c, TokenUser{Roles: map[string]bool{}})
			c.Next()
		},
		isStudentOfCoursePhaseMiddleware(),
		func(c *gin.Context) {
			tu, _ := GetTokenUser(c)
			c.JSON(http.StatusOK, gin.H{
				"isStudentOfCourse":      tu.IsStudentOfCourse,
				"isStudentOfCoursePhase": tu.IsStudentOfCoursePhase,
			})
		},
	)
	return r
}

func newCoreStub(status int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		if body != "" {
			_, _ = w.Write([]byte(body))
		}
	}))
}

// TestStudentPath_CrossCourseDenied is the regression guard for
// GHSA-4wgm-2wvm-fphm: when core denies cross-course access with 403, the
// student-verification middleware must leave IsStudentOfCourse false so the
// auth gate denies the request. Before the fix, a 403 was silently treated as
// success and IsStudentOfCourse was hardcoded to true.
func TestStudentPath_CrossCourseDenied(t *testing.T) {
	core := newCoreStub(http.StatusForbidden, `{"error":"no matching permission found"}`)
	defer core.Close()

	router := newStudentPathRouter(t, core.URL)

	req, _ := http.NewRequest(http.MethodGet, "/api/course_phase/11111111-1111-1111-1111-111111111111/resource", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)

	var body struct {
		IsStudentOfCourse      bool `json:"isStudentOfCourse"`
		IsStudentOfCoursePhase bool `json:"isStudentOfCoursePhase"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	assert.False(t, body.IsStudentOfCourse, "cross-course actor must not be a student of the course")
	assert.False(t, body.IsStudentOfCoursePhase)
}

// TestStudentPath_SameCourseAllowed ensures legitimate same-course students are
// unaffected: core returns 200 and the flags are populated from the response.
func TestStudentPath_SameCourseAllowed(t *testing.T) {
	core := newCoreStub(http.StatusOK, `{"isStudentOfCoursePhase":true,"courseParticipationID":"00000000-0000-0000-0000-000000000001"}`)
	defer core.Close()

	router := newStudentPathRouter(t, core.URL)

	req, _ := http.NewRequest(http.MethodGet, "/api/course_phase/11111111-1111-1111-1111-111111111111/resource", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)

	var body struct {
		IsStudentOfCourse      bool `json:"isStudentOfCourse"`
		IsStudentOfCoursePhase bool `json:"isStudentOfCoursePhase"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	assert.True(t, body.IsStudentOfCourse)
	assert.True(t, body.IsStudentOfCoursePhase)
}

// TestStudentPath_UnexpectedCoreStatusFailsClosed ensures an unexpected core
// status aborts with 500 rather than proceeding to the handler.
func TestStudentPath_UnexpectedCoreStatusFailsClosed(t *testing.T) {
	core := newCoreStub(http.StatusInternalServerError, "")
	defer core.Close()

	router := newStudentPathRouter(t, core.URL)

	req, _ := http.NewRequest(http.MethodGet, "/api/course_phase/11111111-1111-1111-1111-111111111111/resource", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusInternalServerError, resp.Code)
}
