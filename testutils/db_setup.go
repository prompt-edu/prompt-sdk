package testutils

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type TestDB[Q any] struct {
	Conn    *pgxpool.Pool
	Queries Q
}

// SetupTestDB loads one SQL file that defines both schema and data.
// Prefer SetupTestDBWithMigrations, which builds the schema from the service's migrations.
func SetupTestDB[Q any](ctx context.Context, sqlDumpPath string, queryFactory func(*pgxpool.Pool) Q) (*TestDB[Q], func(), error) {
	return setupTestDB(ctx, []string{sqlDumpPath}, queryFactory)
}

// SetupTestDBWithMigrations applies every *.up.sql file in migrationsDir in golang-migrate's version order, then the seed files.
func SetupTestDBWithMigrations[Q any](ctx context.Context, migrationsDir string, queryFactory func(*pgxpool.Pool) Q, seedPaths ...string) (*TestDB[Q], func(), error) {
	migrationPaths, err := upMigrationPaths(migrationsDir)
	if err != nil {
		return nil, nil, err
	}
	return setupTestDB(ctx, append(migrationPaths, seedPaths...), queryFactory)
}

func setupTestDB[Q any](ctx context.Context, sqlPaths []string, queryFactory func(*pgxpool.Pool) Q) (*TestDB[Q], func(), error) {
	// Set up PostgreSQL container
	req := testcontainers.ContainerRequest{
		Image:        "postgres:15",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "testuser",
			"POSTGRES_PASSWORD": "testpass",
			"POSTGRES_DB":       "prompt",
		},
		WaitingFor: wait.ForAll(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
			wait.ForListeningPort("5432/tcp"),
		),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("could not start container: %w", err)
	}

	// Get container's host and port
	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, nil, fmt.Errorf("could not get container host: %w", err)
	}
	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, nil, fmt.Errorf("could not get container port: %w", err)
	}
	dbURL := fmt.Sprintf("postgres://testuser:testpass@%s:%s/prompt?sslmode=disable", host, port.Port())

	conn, err := connect(ctx, dbURL)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, nil, err
	}

	for _, path := range sqlPaths {
		if err := runSQLFile(ctx, conn, path); err != nil {
			conn.Close()
			_ = container.Terminate(ctx)
			return nil, nil, err
		}
	}

	// Create queries using the provided factory function
	queries := queryFactory(conn)

	// Return the TestDB and a cleanup function
	cleanup := func() {
		conn.Close()
		_ = container.Terminate(ctx)
	}

	return &TestDB[Q]{
		Conn:    conn,
		Queries: queries,
	}, cleanup, nil
}

func connect(ctx context.Context, dbURL string) (*pgxpool.Pool, error) {
	var lastErr error
	for range 5 {
		conn, err := pgxpool.New(ctx, dbURL)
		if err == nil {
			if err = conn.Ping(ctx); err == nil {
				return conn, nil
			}
			conn.Close()
		}
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}
	return nil, fmt.Errorf("failed to connect to the database after retries: %w", lastErr)
}

func upMigrationPaths(migrationsDir string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(migrationsDir, "*.up.sql"))
	if err != nil {
		return nil, fmt.Errorf("could not list migrations: %w", err)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no up migrations found in %s", migrationsDir)
	}

	pathByVersion := make(map[uint64]string, len(paths))
	for _, path := range paths {
		version, err := migrationVersion(path)
		if err != nil {
			return nil, err
		}
		if other, ok := pathByVersion[version]; ok {
			return nil, fmt.Errorf("migrations %s and %s share version %d", other, path, version)
		}
		pathByVersion[version] = path
	}

	ordered := make([]string, 0, len(paths))
	for _, version := range slices.Sorted(maps.Keys(pathByVersion)) {
		ordered = append(ordered, pathByVersion[version])
	}
	return ordered, nil
}

func migrationVersion(path string) (uint64, error) {
	prefix, _, _ := strings.Cut(filepath.Base(path), "_")
	version, err := strconv.ParseUint(prefix, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("migration %s has no numeric version prefix", path)
	}
	return version, nil
}

func runSQLFile(ctx context.Context, conn *pgxpool.Pool, path string) error {
	statements, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("could not read SQL file: %w", err)
	}
	if _, err := conn.Exec(ctx, string(statements)); err != nil {
		return fmt.Errorf("failed to execute %s: %w", path, err)
	}
	return nil
}
