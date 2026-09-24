package promptSDK

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prompt-edu/prompt-sdk/audit"
	"github.com/prompt-edu/prompt-sdk/promptTypes"
	"github.com/prompt-edu/prompt-sdk/utils"
	log "github.com/sirupsen/logrus"
)

const (
	coursePhasePath = "/course_phase/:coursePhaseID"
	migrationsPath  = "./db/migration"

	// dbPingTimeout has to cover a cold pgxpool opening its first connection, not just a round trip.
	dbPingTimeout   = 3 * time.Second
	shutdownTimeout = 10 * time.Second
)

// ServiceOptions configures Bootstrap for a single phase service. Only RegisterRoutes genuinely
// varies between services; everything else is identical wiring captured as data.
type ServiceOptions struct {
	// ServiceName is the human-readable service name reported by the /info endpoint (e.g. "assessment").
	ServiceName string

	// BasePath is the router group prefix, verbatim (e.g. "/assessment/api"). Not derived from
	// ServiceName because the existing services are inconsistent (leading slash, naming).
	BasePath string

	// DBEnvPrefix selects the per-phase DB_HOST_<PREFIX>/DB_PORT_<PREFIX> vars (e.g. "ASSESSMENT").
	DBEnvPrefix string

	// DefaultDBPort is the local-development fallback port for this phase (e.g. "5435").
	DefaultDBPort string

	// SentryDSNEnv is the env var holding this service's Sentry DSN (e.g. "SENTRY_DSN_ASSESSMENT").
	SentryDSNEnv string

	// DefaultAddress is the fallback listen address when SERVER_ADDRESS is unset (e.g. "localhost:8085").
	DefaultAddress string

	// Capabilities is reported by the /info endpoint. Use the promptTypes.Capability* keys.
	// Bootstrap adds promptTypes.CapabilityAuditLog itself, reflecting AUDIT_ENABLED.
	Capabilities map[string]bool

	// RegisterRoutes wires the service's own modules onto the router groups. It receives the base
	// api group, the phase-scoped group, and the connection pool. The service builds its own
	// db.New(conn), because the SDK cannot reference a service-specific Queries type.
	RegisterRoutes func(api, coursePhase *gin.RouterGroup, conn *pgxpool.Pool) error
}

func (o ServiceOptions) validate() error {
	for _, field := range []struct{ name, value string }{
		{"ServiceName", o.ServiceName},
		{"BasePath", o.BasePath},
		{"DBEnvPrefix", o.DBEnvPrefix},
		{"DefaultDBPort", o.DefaultDBPort},
		{"DefaultAddress", o.DefaultAddress},
	} {
		if field.value == "" {
			return fmt.Errorf("ServiceOptions.%s is required", field.name)
		}
	}
	if o.RegisterRoutes == nil {
		return errors.New("ServiceOptions.RegisterRoutes is required")
	}
	return nil
}

// Bootstrap composes the phase-service startup sequence that every service copies today:
// Sentry -> DB URL -> migrations -> pgx pool -> gin + Sentry + CORS + audit -> route groups ->
// Keycloak -> service routes -> /info health endpoint -> run. It blocks until the process is
// signalled, and returns an error instead of calling log.Fatal:
//
//	if err := promptSDK.Bootstrap(opts); err != nil {
//	    log.Fatal(err)
//	}
func Bootstrap(opts ServiceOptions) error {
	if err := opts.validate(); err != nil {
		return err
	}

	sentryEnabled := GetEnv("SENTRY_ENABLED", "false") == "true"
	if sentryEnabled {
		_ = utils.InitSentry(GetEnv(opts.SentryDSNEnv, ""))
		defer sentry.Flush(2 * time.Second)
	}

	databaseURL := utils.GetDatabaseURLForPrefix(opts.DBEnvPrefix, opts.DefaultDBPort)

	if err := utils.RunMigrations(databaseURL, migrationsPath); err != nil {
		return err
	}

	conn, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		return fmt.Errorf("unable to create connection pool: %w", err)
	}
	defer conn.Close()

	router := gin.Default()
	if sentryEnabled {
		router.Use(sentrygin.New(sentrygin.Options{}))
	}
	router.Use(CORSMiddleware(GetEnv("CORE_HOST", "http://localhost:3000")))
	router.Use(audit.Middleware(audit.NewCoreSink(utils.GetCoreUrl(), opts.ServiceName), audit.WithSourceService(opts.ServiceName)))

	api := router.Group(opts.BasePath)
	coursePhase := api.Group(coursePhasePath)

	if err := InitPhaseKeycloak(); err != nil {
		return err
	}

	if err := opts.RegisterRoutes(api, coursePhase, conn); err != nil {
		return fmt.Errorf("failed to register routes: %w", err)
	}

	capabilities := make(map[string]bool, len(opts.Capabilities)+1)
	maps.Copy(capabilities, opts.Capabilities)
	capabilities[promptTypes.CapabilityAuditLog] = audit.Enabled()

	promptTypes.RegisterInfoEndpoint(api, promptTypes.ServiceInfo{
		ServiceName:  opts.ServiceName,
		Version:      GetEnv("SERVER_IMAGE_TAG", ""),
		Capabilities: capabilities,
	}, func() bool {
		pingCtx, cancel := context.WithTimeout(context.Background(), dbPingTimeout)
		defer cancel()
		return conn.Ping(pingCtx) == nil
	})

	serverAddress := GetEnv("SERVER_ADDRESS", opts.DefaultAddress)
	if serverAddress == "" {
		// an empty address makes http.Server listen on port 80 instead of failing
		serverAddress = opts.DefaultAddress
	}
	log.Infof("%s server started on %s", opts.ServiceName, serverAddress)
	return serve(router, serverAddress)
}

// serve runs the server until SIGINT or SIGTERM, then drains in-flight requests, so the
// connection pool close and the Sentry flush that Bootstrap defers actually run.
func serve(handler http.Handler, address string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	server := &http.Server{Addr: address, Handler: handler}
	return serveUntil(ctx, server, listener)
}

// serveUntil serves on listener until ctx is done and returns only once Shutdown has finished
// draining in-flight requests.
func serveUntil(ctx context.Context, server *http.Server, listener net.Listener) error {
	ctx, cancel := context.WithCancel(ctx)
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancelShutdown()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Warnf("graceful shutdown failed: %v", err)
		}
	}()

	err := server.Serve(listener)
	// Serve returns ErrServerClosed as soon as Shutdown starts, not when it finishes, so wait for
	// the shutdown goroutine. On any other error, cancel wakes that goroutine so it cannot leak.
	cancel()
	<-shutdownDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
