package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/prompt-edu/prompt-sdk/internal/urlcheck"
	"github.com/prompt-edu/prompt-sdk/promptTypes"
	"github.com/prompt-edu/prompt-sdk/utils"
	log "github.com/sirupsen/logrus"
)

const (
	eventTimeout  = 10 * time.Second
	healthTimeout = 2 * time.Second
	// maxInfoBytes caps the health answer, which is a few hundred bytes.
	maxInfoBytes = 64 << 10
)

var ErrCallNotFound = errors.New("the AI server knows no such call of this service in this course phase")

// Service is a phase service as the AI server knows it: its name and its key in AI_SERVICE_KEYS.
// The name is also the prefix of its features.
type Service struct {
	Name string
	Key  string
}

// Client calls the AI server on behalf of one phase service. Every request carries the user's
// Keycloak token, so the AI server checks the user's role in the course phase as well.
type Client struct {
	host    *url.URL
	service Service
	// No overall timeout: a streamed completion runs as long as the AI server allows it. Calls end
	// with their context.
	httpClient *http.Client
}

// NewClient builds a client for the AI server at host, for example "https://prompt.example.com"
// or "http://server-ai:8080".
func NewClient(host string, service Service) (*Client, error) {
	parsed, err := url.Parse(host)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("the AI server host must be an http or https URL, got %q", host)
	}
	if service.Name == "" || service.Key == "" {
		return nil, errors.New("the service name and key are required")
	}
	if urlcheck.InsecureExternal(host) {
		log.Warnf("ai: %q sends the service key and the provider key over plaintext HTTP to a non-internal host; "+
			"use HTTPS or keep phase->AI traffic on an internal network", host)
	}
	return &Client{host: parsed, service: service, httpClient: &http.Client{}}, nil
}

// NewClientFromEnv builds the client from AI_ENABLED, SERVER_AI_HOST and AI_SERVICE_KEY. It returns
// nil when AI is switched off, and also, with a warning, when the host or key is missing or
// invalid, so a service can treat a nil client as "no AI" and hide every AI surface.
func NewClientFromEnv(serviceName string) *Client {
	if utils.GetEnv("AI_ENABLED", "") != "true" {
		return nil
	}
	client, err := NewClient(utils.GetEnv("SERVER_AI_HOST", ""), Service{Name: serviceName, Key: utils.GetEnv("AI_SERVICE_KEY", "")})
	if err != nil {
		log.Warnf("ai: AI_ENABLED is true, but %s has no AI client: %v", serviceName, err)
		return nil
	}
	return client
}

// BaseURL is the OpenAI-compatible base URL of a course phase, for a stock OpenAI client.
func (c *Client) BaseURL(coursePhaseID uuid.UUID) string {
	return c.coursePhaseURL(coursePhaseID, "v1").String()
}

func (c *Client) coursePhaseURL(coursePhaseID uuid.UUID, path ...string) *url.URL {
	return c.host.JoinPath(append([]string{"ai/api/course_phase", coursePhaseID.String()}, path...)...)
}

// SetHeaders sets the service and call headers on header, for example on the outgoing request of an
// httputil.ReverseProxy. The caller sets the user's Authorization header.
func (c *Client) SetHeaders(header http.Header, call Call) error {
	if err := call.validate(c.service.Name); err != nil {
		return err
	}
	c.setServiceHeaders(header)
	call.setHeaders(header)
	return nil
}

func (c *Client) setServiceHeaders(header http.Header) {
	header.Set(ServiceHeader, c.service.Name)
	header.Set(ServiceTokenHeader, c.service.Key)
}

// Transport sets the service and call headers on every request, for a stock OpenAI client with
// BaseURL as its base URL and the user's token as its API key. A nil base uses
// http.DefaultTransport.
func (c *Client) Transport(base http.RoundTripper, call Call) (http.RoundTripper, error) {
	if base == nil {
		base = http.DefaultTransport
	}
	header := http.Header{}
	if err := c.SetHeaders(header, call); err != nil {
		return nil, err
	}
	return &transport{base: base, header: header}, nil
}

type transport struct {
	base   http.RoundTripper
	header http.Header
}

func (t *transport) RoundTrip(request *http.Request) (*http.Response, error) {
	// A RoundTripper must not modify the caller's request.
	request = request.Clone(request.Context())
	for name, values := range t.header {
		request.Header[name] = values
	}
	return t.base.RoundTrip(request)
}

// ChatCompletion sends an OpenAI chat completion body for a course phase. authHeader is the user's
// Authorization header. The answer comes back unread with any status, so a phase can pass a
// streamed answer and the AI server's errors through; the caller closes its body. CallID reads the
// recorded call from it.
func (c *Client) ChatCompletion(ctx context.Context, coursePhaseID uuid.UUID, authHeader string, call Call, body []byte) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.coursePhaseURL(coursePhaseID, "v1/chat/completions").String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build the chat completion request: %w", err)
	}
	if err := c.SetHeaders(request.Header, call); err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", authHeader)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call the AI server: %w", err)
	}
	return response, nil
}

// PostEvent reports an oversight event on a call this service made. authHeader is the user's
// Authorization header; the AI server records the user as the actor. It answers ErrCallNotFound
// for a call of another course phase or another service.
func (c *Client) PostEvent(ctx context.Context, coursePhaseID, callID uuid.UUID, authHeader string, event EventRequest) error {
	if err := event.Validate(); err != nil {
		return err
	}
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode the event: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, eventTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.coursePhaseURL(coursePhaseID, "calls", callID.String(), "events").String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build the event request: %w", err)
	}
	c.setServiceHeaders(request.Header)
	request.Header.Set("Authorization", authHeader)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("post the %s event for call %s: %w", event.Type, callID, err)
	}
	defer func() { _ = response.Body.Close() }()
	switch response.StatusCode {
	case http.StatusCreated:
		return nil
	case http.StatusNotFound:
		return ErrCallNotFound
	default:
		return fmt.Errorf("the AI server answered %d to the %s event for call %s", response.StatusCode, event.Type, callID)
	}
}

// Healthy reports whether the AI server answers its info endpoint as healthy. A phase shows its AI
// surfaces only while it does.
func (c *Client) Healthy(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.host.JoinPath("ai/api/info").String(), nil)
	if err != nil {
		return false
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return false
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return false
	}
	var info promptTypes.ServiceInfo
	if err := json.NewDecoder(io.LimitReader(response.Body, maxInfoBytes)).Decode(&info); err != nil {
		return false
	}
	return info.Healthy
}
