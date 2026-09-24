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
	stdout := &maskingWriter{out: os.Stdout}
	stderr := &maskingWriter{out: os.Stderr}
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
// line so a connection string straddling two writes cannot slip through unmasked.
type maskingWriter struct {
	out     io.Writer
	pending []byte
}

func (w *maskingWriter) Write(p []byte) (int, error) {
	w.pending = append(w.pending, p...)
	for {
		end := bytes.IndexByte(w.pending, '\n')
		if end < 0 {
			return len(p), nil
		}
		w.emit(w.pending[:end+1])
		w.pending = w.pending[end+1:]
	}
}

func (w *maskingWriter) flush() {
	if len(w.pending) == 0 {
		return
	}
	w.emit(w.pending)
	w.pending = nil
}

func (w *maskingWriter) emit(line []byte) {
	// a lost log line must not abort the migration, as writing to os.Stdout directly never could
	_, _ = io.WriteString(w.out, SanitizeDatabaseURL(string(line)))
}
