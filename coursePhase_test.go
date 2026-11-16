package promptSDK

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchCoursePhase(t *testing.T) {
	phaseID := uuid.New()
	var gotPath, gotAuth string
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"id":"` + phaseID.String() + `","name":"Assessment","coursePhaseTypeName":"Assessment","restrictedData":{"key":"value"}}`))
	}))
	defer core.Close()

	phase, err := FetchCoursePhase(context.Background(), core.URL, "Bearer token", phaseID)

	require.NoError(t, err)
	assert.Equal(t, "/api/course_phases/"+phaseID.String(), gotPath)
	assert.Equal(t, "Bearer token", gotAuth, "the caller's token is forwarded")
	assert.Equal(t, phaseID, phase.ID)
	assert.Equal(t, "Assessment", phase.CoursePhaseTypeName)
	assert.Equal(t, "value", phase.RestrictedData["key"])
}

// A mail template with a pasted image pushes restrictedData past a few MiB.
func TestFetchCoursePhaseDecodesALargePhase(t *testing.T) {
	phaseID := uuid.New()
	template := strings.Repeat("a", 4<<20)
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":             phaseID,
			"restrictedData": map[string]any{"resultsReleasedMail": template},
		})
	}))
	defer core.Close()

	phase, err := FetchCoursePhase(context.Background(), core.URL, "Bearer token", phaseID)

	require.NoError(t, err)
	assert.Equal(t, template, phase.RestrictedData["resultsReleasedMail"])
}

func TestFetchCoursePhaseFailsOnAnythingButOK(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError} {
		core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))

		_, err := FetchCoursePhase(context.Background(), core.URL, "Bearer token", uuid.New())

		assert.Error(t, err, "status %d", status)
		core.Close()
	}
}

func TestFetchCoursePhaseHonorsTheContext(t *testing.T) {
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer core.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := FetchCoursePhase(ctx, core.URL, "Bearer token", uuid.New())

	assert.ErrorIs(t, err, context.Canceled)
}
