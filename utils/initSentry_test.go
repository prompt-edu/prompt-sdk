package utils

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getsentry/sentry-go"
)

func captureSentryRequest(t *testing.T, sendDefaultPII bool) *sentry.Request {
	t.Helper()

	var captured *sentry.Event
	client, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:            "https://key@o0.ingest.example.test/1",
		DataCollection: sentryDataCollection(sendDefaultPII),
		BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			captured = event
			return nil
		},
	})
	if err != nil {
		t.Fatalf("new sentry client: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://example.test/p?q=safe&token=abc", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	req.Header.Set("X-User-Id", "42")
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "session", Value: "s3cret"})

	scope := sentry.NewScope()
	scope.SetRequest(req)
	sentry.NewHub(client, scope).CaptureMessage("probe")

	if captured == nil {
		t.Fatal("no event captured")
	}
	return captured.Request
}

func TestSentryDataCollectionScrubsPIIByDefault(t *testing.T) {
	got := captureSentryRequest(t, false)

	want := map[string]string{
		"X-Forwarded-For": "[Filtered]",
		"X-User-Id":       "[Filtered]",
		"Content-Type":    "application/json",
	}
	for header, expected := range want {
		if got.Headers[header] != expected {
			t.Errorf("header %s = %q, want %q", header, got.Headers[header], expected)
		}
	}

	if got.Cookies != "" {
		t.Errorf("cookies = %q, want empty", got.Cookies)
	}
	if got.QueryString != "q=safe&token=[Filtered]" {
		t.Errorf("query = %q, want token filtered", got.QueryString)
	}
}

func TestSentryDataCollectionSendsPIIWhenEnabled(t *testing.T) {
	got := captureSentryRequest(t, true)

	if got.Headers["X-Forwarded-For"] != "1.2.3.4" {
		t.Errorf("X-Forwarded-For = %q, want raw value", got.Headers["X-Forwarded-For"])
	}
	if got.Cookies == "" {
		t.Error("cookies are empty, want raw value")
	}
	if got.QueryString != "q=safe&token=[Filtered]" {
		t.Errorf("query = %q, want token filtered even with PII enabled", got.QueryString)
	}
}
