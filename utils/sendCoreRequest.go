package utils

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
)

// SendCoreRequest sends an HTTP request to the core service with context support for cancellation and timeout control.
func SendCoreRequest(ctx context.Context, method, authHeader string, body io.Reader, url string) (*http.Response, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		log.Error("Error creating request:", err)
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Error("Error sending request:", err)
		return nil, fmt.Errorf("failed to send HTTP request: %w", err)
	}

	return resp, nil
}
