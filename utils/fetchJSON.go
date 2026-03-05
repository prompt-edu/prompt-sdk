package utils

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// fetchJSON performs an HTTP GET request to the given URL with the auth header,
// checks for a successful response, and returns the body.
func FetchJSON(url, authHeader string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", authHeader)

	resp, err := (&http.Client{
		Timeout: 10 * time.Second,
	}).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-200 response: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
