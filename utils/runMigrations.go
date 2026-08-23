package utils

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// RunMigrations executes database migrations using the golang-migrate tool.
// It accepts the database URL and the path to the migration files. Migrate's output is streamed
// with the database password masked, so the credential never reaches the logs even when migrate
// echoes the connection string.
func RunMigrations(databaseURL, migrationPath string) error {
	password := databasePassword()
	stdout := &maskingWriter{out: os.Stdout, password: password}
	stderr := &maskingWriter{out: os.Stderr, password: password}
	defer stdout.flush()
	defer stderr.flush()

	cmd := exec.Command("migrate", "-path", migrationPath, "-database", databaseURL, "up")
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	return nil
}

// maskingWriter forwards output with the password masked, holding back an incomplete trailing
// line so a password straddling two writes cannot slip through unmasked.
type maskingWriter struct {
	out      io.Writer
	password string
	pending  []byte
}

func (w *maskingWriter) Write(p []byte) (int, error) {
	w.pending = append(w.pending, p...)
	for {
		end := bytes.IndexByte(w.pending, '\n')
		if end < 0 {
			return len(p), nil
		}
		if err := w.emit(w.pending[:end+1]); err != nil {
			return 0, err
		}
		w.pending = w.pending[end+1:]
	}
}

func (w *maskingWriter) flush() {
	if len(w.pending) == 0 {
		return
	}
	_ = w.emit(w.pending)
	w.pending = nil
}

func (w *maskingWriter) emit(line []byte) error {
	_, err := io.WriteString(w.out, SanitizeDatabaseURL(string(line), w.password))
	return err
}
