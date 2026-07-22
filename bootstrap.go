package promptSDK

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prompt-edu/prompt-sdk/promptTypes"
	"github.com/prompt-edu/prompt-sdk/utils"
	log "github.com/sirupsen/logrus"
)

// ServiceOptions configures Bootstrap for a single phase service. Only RegisterRoutes genuinely
// varies between services; everything else is identical wiring captured as data.
type ServiceOptions struct {
	// ServiceName is the human-readable service name reported by the /info endpoint (e.g. "assessment").
	ServiceName string

	// BasePath is the router group prefix, verbatim (e.g. "/assessment/api"). Not derived from
	// ServiceName because the existing services are inconsistent (leading slash, naming).
	BasePath string

	// CoursePhasePath is the sub-group nested under BasePath for phase-scoped routes.
	// Defaults to "/course_phase/:coursePhaseID".
	CoursePhasePath string

	// DBEnvPrefix selects the per-phase DB_HOST_<PREFIX>/DB_PORT_<PREFIX> vars (e.g. "ASSESSMENT").
	DBEnvPrefix string

	// DefaultDBPort is the local-development fallback port for this phase (e.g. "5435").
	DefaultDBPort string

	// SentryDSNEnv is the env var holding this service's Sentry DSN (e.g. "SENTRY_DSN_ASSESSMENT").
	SentryDSNEnv string

	// DefaultAddress is the fallback listen address when SERVER_ADDRESS is unset (e.g. "localhost:8085").
	DefaultAddress string

	// MigrationsPath is the golang-migrate source path. Defaults to "./db/migration".
	MigrationsPath string

	// Capabilities is reported by the /info endpoint. Use the promptTypes.Capability* keys.
	Capabilities map[string]bool

	// RegisterRoutes wires the service's own modules onto the router groups. It receives the base
	// api group, the phase-scoped group, and the connection pool (the service builds its own
	// db.New(conn) — the SDK cannot reference a service-specific Queries type).
	RegisterRoutes func(api, coursePhase *gin.RouterGroup, conn *pgxpool.Pool) error
}

// Bootstrap composes the phase-service startup sequence that every service copies today:
// Sentry -> DB URL -> migrations -> pgx pool -> gin + Sentry + CORS -> route groups -> Keycloak ->
// service routes -> /info health endpoint -> run. It returns an error instead of calling log.Fatal,
// so callers do log.Fatal(promptSDK.Bootstrap(opts)). It blocks in router.Run until the server exits.
func Bootstrap(opts ServiceOptions) error {
	coursePhasePath := opts.CoursePhasePath
	if coursePhasePath == "" {
		coursePhasePath = "/course_phase/:coursePhaseID"
	}
	migrationsPath := opts.MigrationsPath
	if migrationsPath == "" {
		migrationsPath = "./db/migration"
	}

	sentryEnabled := GetEnv("SENTRY_ENABLED", "false") == "true"
	if sentryEnabled {
		_ = utils.InitSentry(GetEnv(opts.SentryDSNEnv, ""))
		defer sentry.Flush(2 * time.Second)
	}

	databaseURL := utils.GetDatabaseURLForPrefix(opts.DBEnvPrefix, opts.DefaultDBPort)

	if err := runMigrations(migrationsPath, databaseURL); err != nil {
		return err
	}

	ctx := context.Background()
	conn, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("unable to create connection pool: %w", err)
	}
	defer conn.Close()

	router := gin.Default()
	if sentryEnabled {
		router.Use(sentrygin.New(sentrygin.Options{}))
	}
	router.Use(CORSMiddleware(GetEnv("CORE_HOST", "http://localhost:3000")))

	api := router.Group(opts.BasePath)
	coursePhase := api.Group(coursePhasePath)

	if err := InitPhaseKeycloak(); err != nil {
		return err
	}

	if opts.RegisterRoutes != nil {
		if err := opts.RegisterRoutes(api, coursePhase, conn); err != nil {
			return fmt.Errorf("failed to register routes: %w", err)
		}
	}

	promptTypes.RegisterInfoEndpoint(api, promptTypes.ServiceInfo{
		ServiceName:  opts.ServiceName,
		Version:      GetEnv("SERVER_IMAGE_TAG", ""),
		Capabilities: opts.Capabilities,
	}, func() bool {
		pingCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer cancel()
		return conn.Ping(pingCtx) == nil
	})

	serverAddress := GetEnv("SERVER_ADDRESS", opts.DefaultAddress)
	log.Infof("%s server started on %s", opts.ServiceName, serverAddress)
	return router.Run(serverAddress)
}

// runMigrations runs golang-migrate and prints its output with the DB password masked, so the
// credential never reaches the logs even when migrate echoes the connection string on error.
func runMigrations(migrationsPath, databaseURL string) error {
	cmd := exec.Command("migrate", "-path", migrationsPath, "-database", databaseURL, "up")
	output, err := cmd.CombinedOutput()
	sanitized := utils.SanitizeDatabaseURL(string(output), GetEnv("DB_PASSWORD", "prompt-postgres"))
	if err != nil {
		return fmt.Errorf("failed to run migrations: %w\n%s", err, sanitized)
	}
	fmt.Print(sanitized)
	return nil
}
