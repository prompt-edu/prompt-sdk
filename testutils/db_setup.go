package testutils

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type TestDB[Q any] struct {
	Conn    *pgxpool.Pool
	Queries Q
}

func SetupTestDB[Q any](ctx context.Context, sqlDumpPath string, queryFactory func(*pgxpool.Pool) Q) (*TestDB[Q], func(), error) {
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

	// Run the SQL dump
	if err := runSQLDump(ctx, conn, sqlDumpPath); err != nil {
		conn.Close()
		_ = container.Terminate(ctx)
		return nil, nil, fmt.Errorf("failed to run SQL dump: %w", err)
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

func runSQLDump(ctx context.Context, conn *pgxpool.Pool, path string) error {
	dump, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("could not read SQL dump file: %w", err)
	}
	_, err = conn.Exec(ctx, string(dump))
	if err != nil {
		return fmt.Errorf("failed to execute SQL dump: %w", err)
	}
	return nil
}
