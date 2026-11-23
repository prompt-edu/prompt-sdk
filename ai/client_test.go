package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testService = Service{Name: "assessment", Key: "assessment-service-key"}

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewClient(server.URL, testService)
	require.NoError(t, err)
	return client
}

func TestNewClient(t *testing.T) {
	for host, valid := range map[string]bool{
		"https://prompt.example.com":   true,
		"http://server-ai:8080":        true,
		"https://prompt.example.com/":  true,
		"server-ai:8080":               false,
		"ftp://prompt.example.com":     false,
		"":                             false,
		"https://":                     false,
		"http://[::1]:namedport/nope ": false,
	} {
		_, err := NewClient(host, testService)
		assert.Equal(t, valid, err == nil, host)
	}

	_, err := NewClient("https://prompt.example.com", Service{Name: "assessment"})
	assert.Error(t, err, "a service without a key")
	_, err = NewClient("https://prompt.example.com", Service{Key: "key"})
	assert.Error(t, err, "a key without a service")
}

func TestNewClientFromEnv(t *testing.T) {
	t.Setenv("SERVER_AI_HOST", "http://server-ai:8080")
	t.Setenv("AI_SERVICE_KEY", "assessment-service-key")

	t.Setenv("AI_ENABLED", "false")
	assert.Nil(t, NewClientFromEnv("assessment"), "AI switched off")

	t.Setenv("AI_ENABLED", "true")
	client := NewClientFromEnv("assessment")
	require.NotNil(t, client)
	assert.Equal(t, testService, client.service)

	t.Setenv("AI_SERVICE_KEY", "")
	assert.Nil(t, NewClientFromEnv("assessment"), "AI on without a service key")

	t.Setenv("AI_SERVICE_KEY", "assessment-service-key")
	t.Setenv("SERVER_AI_HOST", "")
	assert.Nil(t, NewClientFromEnv("assessment"), "AI on without a host")
}

func TestBaseURL(t *testing.T) {
	phaseID := uuid.New()
	for _, host := range []string{"https://prompt.example.com", "https://prompt.example.com/"} {
		client, err := NewClient(host, testService)
		require.NoError(t, err)
		assert.Equal(t, "https://prompt.example.com/ai/api/course_phase/"+phaseID.String()+"/v1", client.BaseURL(phaseID))
	}
}

func TestSetHeaders(t *testing.T) {
	client, err := NewClient("https://prompt.example.com", testService)
	require.NoError(t, err)

	header := http.Header{}
	require.NoError(t, client.SetHeaders(header, validCall()))
	assert.Equal(t, "assessment", header.Get(ServiceHeader))
	assert.Equal(t, "assessment-service-key", header.Get(ServiceTokenHeader))
	assert.Equal(t, "logos-key-1234", header.Get(ProviderKeyHeader))
	assert.Equal(t, "assessment.action_item_suggestions", header.Get(FeatureHeader))

	header = http.Header{}
	invalid := validCall()
	invalid.Feature = "interview.summary"
	assert.ErrorIs(t, client.SetHeaders(header, invalid), ErrInvalidFeature)
	assert.Empty(t, header, "an invalid call sets nothing")
}

func TestTransport(t *testing.T) {
	var got http.Header
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	})
	transport, err := client.Transport(nil, validCall())
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodGet, client.BaseURL(uuid.New())+"/models", nil)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer user-token")
	response, err := (&http.Client{Transport: transport}).Do(request)
	require.NoError(t, err)
	_ = response.Body.Close()

	assert.Equal(t, "Bearer user-token", got.Get("Authorization"))
	assert.Equal(t, "assessment", got.Get(ServiceHeader))
	assert.Equal(t, "assessment-service-key", got.Get(ServiceTokenHeader))
	assert.Equal(t, "logos-key-1234", got.Get(ProviderKeyHeader))
	assert.Equal(t, "assessment.action_item_suggestions", got.Get(FeatureHeader))
	assert.Empty(t, request.Header.Get(ServiceHeader), "the caller's request stays unchanged")

	invalid := validCall()
	invalid.ProviderKey = ""
	_, err = client.Transport(nil, invalid)
	assert.ErrorIs(t, err, ErrInvalidProviderKey)
}

func TestChatCompletion(t *testing.T) {
	phaseID, callID, subject := uuid.New(), uuid.New(), uuid.New()
	body := []byte(`{"model":"prompt-test-model","stream":true,"messages":[{"role":"user","content":"Summarize"}]}`)
	var gotPath, gotMethod string
	var gotHeader http.Header
	var gotBody []byte
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod, gotHeader = r.URL.Path, r.Method, r.Header.Clone()
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set(CallIDHeader, callID.String())
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})

	call := validCall()
	call.Template, call.TemplateVersion, call.Subjects = "action-items", "1", []uuid.UUID{subject}
	response, err := client.ChatCompletion(context.Background(), phaseID, "Bearer user-token", call, body)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/ai/api/course_phase/"+phaseID.String()+"/v1/chat/completions", gotPath)
	assert.Equal(t, body, gotBody)
	assert.Equal(t, "Bearer user-token", gotHeader.Get("Authorization"))
	assert.Equal(t, "application/json", gotHeader.Get("Content-Type"))
	assert.Equal(t, "assessment", gotHeader.Get(ServiceHeader))
	assert.Equal(t, "assessment-service-key", gotHeader.Get(ServiceTokenHeader))
	assert.Equal(t, "logos-key-1234", gotHeader.Get(ProviderKeyHeader))
	assert.Equal(t, "action-items", gotHeader.Get(TemplateHeader))
	assert.Equal(t, "1", gotHeader.Get(TemplateVersionHeader))
	assert.Equal(t, subject.String(), gotHeader.Get(SubjectsHeader))

	gotCallID, ok := CallID(response.Header)
	require.True(t, ok)
	assert.Equal(t, callID, gotCallID)
	stream, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Equal(t, "data: [DONE]\n\n", string(stream), "the answer comes back unread")
}

func TestChatCompletionPassesErrorsThrough(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"the model is not allowed"}`)
	})
	response, err := client.ChatCompletion(context.Background(), uuid.New(), "Bearer user-token", validCall(), []byte(`{}`))
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
}

func TestChatCompletionRejectsInvalidCall(t *testing.T) {
	requests := 0
	client := newTestClient(t, func(http.ResponseWriter, *http.Request) { requests++ })
	invalid := validCall()
	invalid.Template = "not a label"
	_, err := client.ChatCompletion(context.Background(), uuid.New(), "Bearer user-token", invalid, []byte(`{}`))
	assert.ErrorIs(t, err, ErrInvalidLabel)
	assert.Zero(t, requests, "an invalid call is never sent")
}

func TestPostEvent(t *testing.T) {
	phaseID, callID, target := uuid.New(), uuid.New(), uuid.New()
	distance := 7
	var gotPath string
	var gotHeader http.Header
	var gotBody map[string]any
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotHeader = r.URL.Path, r.Header.Clone()
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
	})

	err := client.PostEvent(context.Background(), phaseID, callID, "Bearer user-token",
		EventRequest{Type: EventEdited, EditDistance: &distance, TargetID: &target})
	require.NoError(t, err)
	assert.Equal(t, "/ai/api/course_phase/"+phaseID.String()+"/calls/"+callID.String()+"/events", gotPath)
	assert.Equal(t, "Bearer user-token", gotHeader.Get("Authorization"))
	assert.Equal(t, "assessment", gotHeader.Get(ServiceHeader))
	assert.Equal(t, "assessment-service-key", gotHeader.Get(ServiceTokenHeader))
	assert.Empty(t, gotHeader.Get(ProviderKeyHeader), "events carry no provider key")
	assert.Equal(t, map[string]any{"type": "edited", "editDistance": float64(7), "targetId": target.String()}, gotBody)
}

func TestPostEventAnswers(t *testing.T) {
	for status, want := range map[int]error{
		http.StatusNotFound:            ErrCallNotFound,
		http.StatusUnauthorized:        nil,
		http.StatusInternalServerError: nil,
	} {
		client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })
		err := client.PostEvent(context.Background(), uuid.New(), uuid.New(), "Bearer user-token", EventRequest{Type: EventShown})
		require.Error(t, err, status)
		if want != nil {
			assert.ErrorIs(t, err, want)
		} else {
			assert.NotErrorIs(t, err, ErrCallNotFound)
		}
	}
}

func TestPostEventRejectsInvalidEvent(t *testing.T) {
	requests := 0
	client := newTestClient(t, func(http.ResponseWriter, *http.Request) { requests++ })
	err := client.PostEvent(context.Background(), uuid.New(), uuid.New(), "Bearer user-token", EventRequest{Type: EventEdited})
	assert.ErrorIs(t, err, ErrInvalidEvent)
	assert.Zero(t, requests)
}

// The AI server binds the body with gin, so the tags must accept what PostEvent sends.
func TestEventRequestBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	distance, target := 3, uuid.New()
	for body, valid := range map[string]bool{
		`{"type":"shown"}`: true,
		`{"type":"edited","editDistance":3,"targetId":"` + target.String() + `"}`: true,
		`{"type":"content_viewed"}`:              false,
		`{"type":"edited","editDistance":-1}`:    false,
		`{"editDistance":3}`:                     false,
		`{"type":"accepted","targetId":"no-id"}`: false,
	} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		var event EventRequest
		err := c.ShouldBindJSON(&event)
		assert.Equal(t, valid, err == nil, body)
		if body == `{"type":"edited","editDistance":3,"targetId":"`+target.String()+`"}` {
			assert.Equal(t, EventRequest{Type: EventEdited, EditDistance: &distance, TargetID: &target}, event)
		}
	}
}

func TestHealthy(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		want   bool
	}{
		"healthy":            {http.StatusOK, `{"serviceName":"ai","healthy":true}`, true},
		"database down":      {http.StatusOK, `{"serviceName":"ai","healthy":false}`, false},
		"error status":       {http.StatusBadGateway, `{"healthy":true}`, false},
		"not the info route": {http.StatusOK, `<html></html>`, false},
	} {
		var gotPath string
		client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			w.WriteHeader(tc.status)
			_, _ = io.WriteString(w, tc.body)
		})
		assert.Equal(t, tc.want, client.Healthy(context.Background()), name)
		assert.Equal(t, "/ai/api/info", gotPath, name)
	}

	unreachable, err := NewClient("http://127.0.0.1:1", testService)
	require.NoError(t, err)
	assert.False(t, unreachable.Healthy(context.Background()))
}
