package promptSDK

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func validServiceOptions() ServiceOptions {
	return ServiceOptions{
		ServiceName:    "assessment",
		BasePath:       "/assessment/api",
		DBEnvPrefix:    "ASSESSMENT",
		DefaultDBPort:  "5435",
		DefaultAddress: "localhost:8085",
		RegisterRoutes: func(api, coursePhase *gin.RouterGroup, conn *pgxpool.Pool) error { return nil },
	}
}

func TestServiceOptionsValidate(t *testing.T) {
	require.NoError(t, validServiceOptions().validate())

	for _, tc := range []struct {
		field  string
		mutate func(*ServiceOptions)
	}{
		{"ServiceName", func(o *ServiceOptions) { o.ServiceName = "" }},
		{"BasePath", func(o *ServiceOptions) { o.BasePath = "" }},
		{"DBEnvPrefix", func(o *ServiceOptions) { o.DBEnvPrefix = "" }},
		{"DefaultDBPort", func(o *ServiceOptions) { o.DefaultDBPort = "" }},
		{"DefaultAddress", func(o *ServiceOptions) { o.DefaultAddress = "" }},
		{"RegisterRoutes", func(o *ServiceOptions) { o.RegisterRoutes = nil }},
	} {
		opts := validServiceOptions()
		tc.mutate(&opts)
		require.ErrorContains(t, opts.validate(), tc.field, "a missing %s must fail startup", tc.field)
	}
}

func TestServeUntilDrainsInFlightRequests(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	started := make(chan struct{})
	release := make(chan struct{})
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusNoContent)
	})}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- serveUntil(ctx, server, listener) }()

	responded := make(chan int, 1)
	go func() {
		resp, err := http.Get("http://" + listener.Addr().String())
		if err != nil {
			responded <- 0
			return
		}
		_ = resp.Body.Close()
		responded <- resp.StatusCode
	}()

	<-started
	cancel()

	select {
	case err := <-served:
		t.Fatalf("serveUntil returned (%v) while a request was still in flight", err)
	case <-time.After(200 * time.Millisecond):
	}

	close(release)
	require.Equal(t, http.StatusNoContent, <-responded, "the in-flight request must complete during shutdown")
	select {
	case err := <-served:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("serveUntil did not return after the in-flight request finished")
	}
}

func TestServeUntilReturnsServeErrorWithoutSignal(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, listener.Close())

	served := make(chan error, 1)
	go func() { served <- serveUntil(context.Background(), &http.Server{}, listener) }()

	select {
	case err := <-served:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("serveUntil blocked on a Serve error that was not caused by shutdown")
	}
}

func TestServeFailsWhenAddressIsInUse(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()

	require.Error(t, serve(http.NotFoundHandler(), listener.Addr().String()))
}
