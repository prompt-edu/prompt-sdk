package keycloakTokenVerifier

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOnlyContainsAdminAndLecturer(t *testing.T) {
	tests := []struct {
		name string
		set  map[string]struct{}
		want bool
	}{
		{
			name: "nil map",
			set:  nil,
			want: true,
		},
		{
			name: "empty map",
			set:  map[string]struct{}{},
			want: true,
		},
		{
			name: "only admin",
			set:  map[string]struct{}{PromptAdmin: {}},
			want: true,
		},
		{
			name: "only lecturer",
			set:  map[string]struct{}{PromptLecturer: {}},
			want: true,
		},
		{
			name: "admin and lecturer",
			set:  map[string]struct{}{PromptAdmin: {}, PromptLecturer: {}},
			want: true,
		},
		{
			name: "contains other role",
			set:  map[string]struct{}{PromptAdmin: {}, "SomeOtherRole": {}},
			want: false,
		},
		{
			name: "only other role",
			set:  map[string]struct{}{"SomeOtherRole": {}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := onlyContainsAdminAndLecturer(tt.set)
			if got != tt.want {
				t.Errorf("onlyContainsAdminAndLecturer(%v) = %v, want %v", tt.set, got, tt.want)
			}
		})
	}
}

func TestContainsCustomRoleName(t *testing.T) {
	tests := []struct {
		name  string
		roles []string
		want  bool
	}{
		{"no roles", []string{}, false},
		{"only built-in", []string{PromptAdmin, PromptLecturer, CourseLecturer, CourseEditor, CourseStudent}, false},
		{"single custom", []string{"CustomRole"}, true},
		{"mix built-in and custom", []string{PromptAdmin, "X"}, true},
		{"multiple customs", []string{"X", "Y"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := containsCustomRoleName(tt.roles...)
			if got != tt.want {
				t.Errorf("containsCustomRoleName(%v) = %v; want %v", tt.roles, got, tt.want)
			}
		})
	}
}

func TestRequiresLecturerOrCustom(t *testing.T) {
	tests := []struct {
		name       string
		allowedSet map[string]struct{}
		roles      []string
		want       bool
	}{
		{"empty set, no roles", nil, []string{}, false},
		{"only lecturer allowed", map[string]struct{}{CourseLecturer: {}}, []string{}, true},
		{"only editor allowed", map[string]struct{}{CourseEditor: {}}, []string{}, true},
		{"lecturer and editor", map[string]struct{}{CourseLecturer: {}, CourseEditor: {}}, []string{}, true},
		{"no lec/editor but custom role in roles", map[string]struct{}{}, []string{"Custom"}, true},
		{"no lec/editor and no custom", map[string]struct{}{}, []string{PromptAdmin, CourseStudent}, false},
		{"editor but custom roles ignored", map[string]struct{}{CourseEditor: {}}, []string{"Whatever"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := requiresLecturerOrCustom(tt.allowedSet, tt.roles)
			if got != tt.want {
				t.Errorf("requiresLecturerOrCustom(%v, %v) = %v; want %v", tt.allowedSet, tt.roles, got, tt.want)
			}
		})
	}
}

func TestAuthorizationMiddleware_StatusCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const roleMapping = `{"courseLecturerRole":"WS24-Lecturer","courseEditorRole":"WS24-Editor","customRolePrefix":"WS24-cg-"}`

	tests := []struct {
		name         string
		allowedRoles []string
		userRoles    []string
		coreStatus   int // 500 where core must not be consulted
		coreBody     string
		want         int
	}{
		{"admin-only route grants admin", []string{PromptAdmin}, []string{PromptAdmin}, http.StatusInternalServerError, "", http.StatusOK},
		{"admin-only route forbids other users", []string{PromptAdmin}, nil, http.StatusInternalServerError, "", http.StatusForbidden},
		{"lecturer route grants course lecturer", []string{CourseLecturer}, []string{"WS24-Lecturer"}, http.StatusOK, roleMapping, http.StatusOK},
		{"lecturer route forbids non-lecturer", []string{CourseLecturer}, nil, http.StatusOK, roleMapping, http.StatusForbidden},
		{"lecturer route keeps 401 when core rejects the token", []string{CourseLecturer}, nil, http.StatusUnauthorized, "", http.StatusUnauthorized},
		{"student route grants same-course student", []string{CourseStudent}, nil, http.StatusOK, `{"isStudentOfCoursePhase":true}`, http.StatusOK},
		{"student route forbids cross-course student", []string{CourseStudent}, nil, http.StatusForbidden, `{"error":"Access denied"}`, http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := newCoreStub(tt.coreStatus, tt.coreBody)
			defer core.Close()
			coreURL, err := url.Parse(core.URL)
			require.NoError(t, err)
			prev := KeycloakTokenVerifierSingleton
			KeycloakTokenVerifierSingleton = &KeycloakTokenVerifier{CoreURL: *coreURL}
			t.Cleanup(func() { KeycloakTokenVerifierSingleton = prev })

			roles := map[string]bool{}
			for _, role := range tt.userRoles {
				roles[role] = true
			}
			r := gin.New()
			r.GET("/api/course_phase/:coursePhaseID/resource",
				func(c *gin.Context) { SetTokenUser(c, TokenUser{Roles: roles}) },
				authorizationMiddleware(tt.allowedRoles...),
				func(c *gin.Context) { c.Status(http.StatusOK) },
			)

			req := httptest.NewRequest(http.MethodGet, "/api/course_phase/11111111-1111-1111-1111-111111111111/resource", nil)
			req.Header.Set("Authorization", "Bearer token")
			resp := httptest.NewRecorder()
			r.ServeHTTP(resp, req)

			assert.Equal(t, tt.want, resp.Code)
		})
	}
}
