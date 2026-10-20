package serviceAuth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseKeysSkipsEmptyAndSupportsRotation(t *testing.T) {
	keys := ParseKeys("interview:old, interview:new, assessment:sekret, broken, empty:, :orphan")

	require.Equal(t, []string{"old", "new"}, keys["interview"])
	require.Equal(t, []string{"sekret"}, keys["assessment"])
	assert.NotContains(t, keys, "empty")
	assert.NotContains(t, keys, "broken")
	assert.NotContains(t, keys, "")
}

func TestMatchesAnyKey(t *testing.T) {
	accepted := []string{"old", "new"}
	assert.True(t, matchesAnyKey("old", accepted))
	assert.True(t, matchesAnyKey("new", accepted))
	assert.False(t, matchesAnyKey("wrong", accepted))
	assert.False(t, matchesAnyKey("", nil))
	assert.False(t, matchesAnyKey("", []string{}))
}

func TestMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/", Middleware(ParseKeys("interview:sekret"), "X-Service", "X-Token"), func(c *gin.Context) {
		c.String(http.StatusOK, ServiceName(c))
	})

	for _, tt := range []struct {
		name, service, token string
		status               int
	}{
		{"valid", "interview", "sekret", http.StatusOK},
		{"wrong key", "interview", "wrong", http.StatusUnauthorized},
		{"another service's key", "assessment", "sekret", http.StatusUnauthorized},
		{"no headers", "", "", http.StatusUnauthorized},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Header.Set("X-Service", tt.service)
			request.Header.Set("X-Token", tt.token)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			require.Equal(t, tt.status, recorder.Code)
			if tt.status == http.StatusOK {
				require.Equal(t, "interview", recorder.Body.String())
			}
		})
	}
}
