package utils

import (
	"fmt"
	"os"
	"os/exec"
)

// RunMigrations executes database migrations using the golang-migrate tool.
// It accepts the database URL and the path to the migration files.
func RunMigrations(databaseURL, migrationPath string) error {
	cmd := exec.Command("migrate", "-path", migrationPath, "-database", databaseURL, "up")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	return nil
}
