package promptSDK

import (
	"testing"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/prompt-edu/prompt-sdk/promptTypes"
	"github.com/stretchr/testify/assert"
)

func TestCoursePhaseParticipationsWithResolutionsValidation(t *testing.T) {
	// Get validator engine
	v, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		t.Fatal("could not get validator engine")
	}

	// Create valid UUID for testing
	validUUID := uuid.MustParse("f47ac10b-58cc-4372-a567-0e02b2c3d479")

	// Create test cases
	tests := []struct {
		name        string
		resolutions []Resolution
		wantError   bool
		errorField  string
	}{
		{
			name: "Valid resolutions",
			resolutions: []Resolution{
				{
					DtoName:       "TestDTO",
					BaseURL:       "https://example.com",
					EndpointPath:  "/api/endpoint",
					CoursePhaseID: validUUID,
				},
			},
			wantError: false,
		},
		{
			name: "Missing DtoName",
			resolutions: []Resolution{
				{
					DtoName:       "", // Missing required field
					BaseURL:       "https://example.com",
					EndpointPath:  "/api/endpoint",
					CoursePhaseID: validUUID,
				},
			},
			wantError:  true,
			errorField: "DtoName",
		},
		{
			name: "Invalid BaseURL",
			resolutions: []Resolution{
				{
					DtoName:       "TestDTO",
					BaseURL:       "invalid-url", // Not a valid URL
					EndpointPath:  "/api/endpoint",
					CoursePhaseID: validUUID,
				},
			},
			wantError:  true,
			errorField: "BaseURL",
		},
		{
			name: "Missing EndpointPath",
			resolutions: []Resolution{
				{
					DtoName:       "TestDTO",
					BaseURL:       "https://example.com",
					EndpointPath:  "", // Missing required field
					CoursePhaseID: validUUID,
				},
			},
			wantError:  true,
			errorField: "EndpointPath",
		},
		{
			name: "Invalid CoursePhaseID",
			resolutions: []Resolution{
				{
					DtoName:       "TestDTO",
					BaseURL:       "https://example.com",
					EndpointPath:  "/api/endpoint",
					CoursePhaseID: uuid.UUID{}, // Zero UUID, invalid
				},
			},
			wantError:  true,
			errorField: "CoursePhaseID",
		},
		{
			name: "Multiple invalid fields",
			resolutions: []Resolution{
				{
					DtoName:       "",            // Invalid
					BaseURL:       "invalid-url", // Invalid
					EndpointPath:  "",            // Invalid
					CoursePhaseID: uuid.UUID{},   // Invalid
				},
			},
			wantError: true,
			// Multiple errors expected
		},
		{
			name: "Multiple resolutions with one invalid",
			resolutions: []Resolution{
				{
					DtoName:       "TestDTO1",
					BaseURL:       "https://example.com",
					EndpointPath:  "/api/endpoint1",
					CoursePhaseID: validUUID,
				},
				{
					DtoName:       "", // Invalid
					BaseURL:       "https://example.com",
					EndpointPath:  "/api/endpoint2",
					CoursePhaseID: validUUID,
				},
			},
			wantError:  true,
			errorField: "DtoName",
		},
	}

	// Run tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Create a struct to validate
			cpp := CoursePhaseParticipationsWithResolutions{
				Participations: []promptTypes.CoursePhaseParticipationWithStudent{},
				Resolutions:    tt.resolutions,
			}

			// Validate the struct
			err := v.Struct(cpp)

			// Check if error matches expectation
			if tt.wantError {
				assert.Error(t, err)
				if tt.errorField != "" {
					// Check if the error contains the expected field
					assert.Contains(t, err.Error(), tt.errorField)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetEndpointPath(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple path", "path", "path"},
		{"leading slash", "/path", "path"},
		{"trailing slash", "path/", "path"},
		{"leading and trailing slash", "/path/", "path"},
		{"nested with slashes", "//nested/path//", "nested/path"},
		{"empty string", "", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := getEndpointPath(c.input)
			assert.Equal(t, c.expected, got)
		})
	}
}

func TestEmptyResolutions(t *testing.T) {
	// Get validator engine
	v, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		t.Fatal("could not get validator engine")
	}

	// Test with empty resolutions slice
	cpp := CoursePhaseParticipationsWithResolutions{
		Participations: []promptTypes.CoursePhaseParticipationWithStudent{},
		Resolutions:    []Resolution{},
	}

	// Empty slice should be valid since dive only applies to elements within the slice
	err := v.Struct(cpp)
	assert.NoError(t, err)
}

func TestNilResolutions(t *testing.T) {
	// Get validator engine
	v, ok := binding.Validator.Engine().(*validator.Validate)
	if !ok {
		t.Fatal("could not get validator engine")
	}

	// Test with nil resolutions slice
	cpp := CoursePhaseParticipationsWithResolutions{
		Participations: []promptTypes.CoursePhaseParticipationWithStudent{},
		Resolutions:    nil,
	}

	// Nil slice should be valid since dive only applies to elements within the slice
	err := v.Struct(cpp)
	assert.NoError(t, err)
}

func TestBuildURL_NoExtraPaths(t *testing.T) {
	id := uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")
	res := Resolution{
		BaseURL:       "https://example-prompt.com/api",
		CoursePhaseID: id,
		EndpointPath:  "/my-endpoint/",
	}
	got, err := buildURL(res)
	assert.NoError(t, err)
	want := "https://example-prompt.com/api/course_phase/123e4567-e89b-12d3-a456-426614174000/my-endpoint"
	assert.Equal(t, want, got)
}

func TestBuildURL_WithExtraPaths(t *testing.T) {
	id := uuid.MustParse("00000000-0000-0000-0000-000000000000")
	res := Resolution{
		BaseURL:       "http://localhost:8080/v1",
		CoursePhaseID: id,
		EndpointPath:  "endpoint",
	}
	got, err := buildURL(res, "p1", "details")
	assert.NoError(t, err)
	want := "http://localhost:8080/v1/course_phase/00000000-0000-0000-0000-000000000000/endpoint/p1/details"
	assert.Equal(t, want, got)
}

func TestBuildURL_WithInvalidBaseURL(t *testing.T) {
	// Test with an invalid URL that would cause issues
	res := Resolution{
		BaseURL:       ":%invalid",
		CoursePhaseID: uuid.New(),
		EndpointPath:  "endpoint",
	}
	got, err := buildURL(res)
	// Verify the function gracefully handles invalid URLs
	assert.Error(t, err)
	assert.Empty(t, got)
}

// TestBuildURL_RejectsUntrustedTargets covers the targets buildURL must refuse,
// because reaching them would forward the caller's bearer token to a host the
// SDK never meant to talk to.
func TestBuildURL_RejectsUntrustedTargets(t *testing.T) {
	const validBase = "https://example-prompt.com/api"
	cases := []struct {
		name         string
		baseURL      string
		endpointPath string
		extraPaths   []string
	}{
		{name: "no scheme", baseURL: "example-prompt.com/api", endpointPath: "endpoint"},
		{name: "scheme relative", baseURL: "//example-prompt.com/api", endpointPath: "endpoint"},
		{name: "file scheme", baseURL: "file:///etc/passwd", endpointPath: "endpoint"},
		{name: "gopher scheme", baseURL: "gopher://example-prompt.com", endpointPath: "endpoint"},
		{name: "empty base URL", baseURL: "", endpointPath: "endpoint"},
		{name: "no host", baseURL: "https:///api", endpointPath: "endpoint"},
		{name: "embedded credentials", baseURL: "https://user:pw@example-prompt.com", endpointPath: "endpoint"},
		{name: "path traversal", baseURL: validBase, endpointPath: "../../../admin"},
		{name: "path traversal mid path", baseURL: validBase, endpointPath: "endpoint/../../admin"},
		{name: "encoded dot segments", baseURL: validBase, endpointPath: "%2e%2e/%2e%2e/admin"},
		{name: "encoded separators", baseURL: validBase, endpointPath: "..%2f..%2fadmin"},
		{name: "invalid escape", baseURL: validBase, endpointPath: "%zz"},
		{name: "traversal in extra path", baseURL: validBase, endpointPath: "endpoint", extraPaths: []string{".."}},
		{name: "encoded traversal in extra path", baseURL: validBase, endpointPath: "endpoint", extraPaths: []string{"%2e%2e"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := buildURL(Resolution{
				BaseURL:       c.baseURL,
				CoursePhaseID: uuid.New(),
				EndpointPath:  c.endpointPath,
			}, c.extraPaths...)
			assert.Error(t, err)
			assert.Empty(t, got)
		})
	}
}

func TestResolveParticipation_RejectsUntrustedTarget(t *testing.T) {
	got, err := ResolveParticipation("Bearer test-token", Resolution{
		DtoName:       "scoreLevel",
		BaseURL:       "file:///etc/passwd",
		EndpointPath:  "endpoint",
		CoursePhaseID: uuid.New(),
	}, uuid.New())
	assert.Error(t, err)
	assert.Nil(t, got)
}
