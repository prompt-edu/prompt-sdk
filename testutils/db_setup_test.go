package testutils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestSetupTestDBWithMigrations(t *testing.T) {
	testDB, cleanup, err := SetupTestDBWithMigrations(t.Context(), "testdata/migrations", func(conn *pgxpool.Pool) *pgxpool.Pool { return conn }, "testdata/seed.sql")
	require.NoError(t, err)
	defer cleanup()

	var name string
	var archived bool
	err = testDB.Queries.QueryRow(t.Context(), "SELECT name, archived FROM course WHERE id = 1").Scan(&name, &archived)
	require.NoError(t, err)
	require.Equal(t, "iPraktikum", name)
	require.False(t, archived)
}

func TestUpMigrationPathsRejects(t *testing.T) {
	tests := map[string][]string{
		"empty directory":      nil,
		"only down migrations": {"1_a.down.sql"},
		"no numeric version":   {"schema.up.sql"},
		"duplicate version":    {"1_a.up.sql", "01_b.up.sql"},
	}
	for name, files := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			for _, file := range files {
				require.NoError(t, os.WriteFile(filepath.Join(dir, file), nil, 0o600))
			}

			_, err := upMigrationPaths(dir)

			require.Error(t, err)
		})
	}
}

func TestConnectReturnsPingError(t *testing.T) {
	conn, err := connect(t.Context(), "postgres://testuser:testpass@127.0.0.1:1/prompt?sslmode=disable")

	require.Error(t, err)
	require.Nil(t, conn)
}
