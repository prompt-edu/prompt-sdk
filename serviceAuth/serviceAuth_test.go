package serviceAuth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
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

func TestParseKeysWarnsWithoutLoggingKeys(t *testing.T) {
	hook := logtest.NewGlobal()
	defer hook.Reset()

	ParseKeys("interview=abc, :orphan, empty:, assessment:shared, matching:shared, interview:old, interview:old, ")

	var messages []string
	for _, entry := range hook.AllEntries() {
		require.Equal(t, log.WarnLevel, entry.Level)
		messages = append(messages, entry.Message)
	}
	require.Equal(t, []string{
		"serviceAuth: skipping entry 1, which is not in the form service:key",
		"serviceAuth: skipping entry 2, which names no service",
		`serviceAuth: skipping entry 3 for service "empty", which has an empty key`,
		`serviceAuth: services "assessment" and "matching" share a key, so either can authenticate as the other`,
	}, messages, "a key repeated under the same service and a trailing comma are not warned about")
	for _, message := range messages {
		for _, secret := range []string{"abc", "orphan", "shared", "old"} {
			assert.NotContains(t, message, secret)
		}
	}
}

func TestParseKeysDoesNotWarnAboutAnEmptyConfiguration(t *testing.T) {
	hook := logtest.NewGlobal()
	defer hook.Reset()

	assert.Empty(t, ParseKeys(""))
	assert.Empty(t, hook.AllEntries())
}

func TestServiceNameReportsAMissingMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	service, ok := ServiceName(c)

	assert.False(t, ok)
	assert.Empty(t, service)
}

func TestMatchesAnyKey(t *testing.T) {
	accepted := []string{"old", "new"}
	assert.True(t, matchesAnyKey("old", accepted))
	assert.True(t, matchesAnyKey("new", accepted))
	assert.False(t, matchesAnyKey("wrong", accepted))
	assert.False(t, matchesAnyKey("ol", accepted))
	assert.False(t, matchesAnyKey("", nil))
	assert.False(t, matchesAnyKey("", []string{}))
}

func TestMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/", Middleware(ParseKeys("interview:sekret"), "X-Service", "X-Token"), func(c *gin.Context) {
		service, ok := ServiceName(c)
		require.True(t, ok)
		c.String(http.StatusOK, service+c.GetHeader("X-Token"))
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
				require.Equal(t, "interview", recorder.Body.String(), "the token header is removed after a match")
			}
		})
	}
}

func TestMiddlewareRejectsEmptyTokenForHandBuiltEmptyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/", Middleware(Keys{"interview": {""}}, "X-Service", "X-Token"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-Service", "interview")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
}
