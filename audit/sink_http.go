package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/prompt-edu/prompt-sdk/utils"
)

// coreSink ships audit events from a phase service to the core ingest endpoint,
// authenticated with the phase's own shared secret.
type coreSink struct {
	url     string // full ingest URL, e.g. https://core/api/audit
	service string // this service's name, sent as X-Audit-Service
	key     string // this service's shared secret
	client  *http.Client
}

// NewCoreSink builds the HTTP sink used by phase services to report audit
// events to core. It is authenticated with a per-service shared secret read
// from AUDIT_INGEST_KEY (no Keycloak on the ingest path). It returns nil — which
// makes Middleware a no-op — when auditing is disabled or no key is configured,
// so callers can wire it in unconditionally.
func NewCoreSink(coreURL, serviceName string) Sink {
	if !Enabled() {
		return nil
	}
	key := utils.GetEnv("AUDIT_INGEST_KEY", "")
	if key == "" {
		return nil
	}
	if insecureExternalURL(coreURL) {
		log.Warnf("audit: ingest URL %q sends the shared secret and event payload over plaintext HTTP to a non-internal host; "+
			"use HTTPS or keep phase->core traffic on an internal network", coreURL)
	}
	return &coreSink{
		url:     coreURL + "/api/audit",
		service: serviceName,
		key:     key,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// insecureExternalURL reports whether raw sends data unencrypted (plain HTTP) to
// a host that is not clearly internal. Loopback, private IPs, and short service
// names (docker/k8s, e.g. "server-core") are treated as internal/trusted and do
// not warn; a dotted public hostname or public IP over HTTP does.
func insecureExternalURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "https" {
		return false
	}
	host := u.Hostname()
	if host == "" || host == "localhost" || !strings.Contains(host, ".") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return !ip.IsLoopback() && !ip.IsPrivate()
	}
	return true
}

// Record posts the event to core with limited retries. It is invoked from a
// background goroutine by the middleware, so blocking here does not affect the
// user request.
func (s *coreSink) Record(ctx context.Context, e Event) error {
	body, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("failed to marshal audit event: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("failed to build audit request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Audit-Service", s.service)
		req.Header.Set("X-Audit-Token", s.key)

		resp, err := s.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode < 300 {
			return nil
		}
		lastErr = fmt.Errorf("audit ingest returned status %d", resp.StatusCode)
	}
	return lastErr
}
