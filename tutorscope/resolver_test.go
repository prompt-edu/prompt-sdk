package tutorscope

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const schemaDDL = `
CREATE TABLE team (
    id              uuid NOT NULL,
    course_phase_id uuid NOT NULL,
    name            text NOT NULL,
    PRIMARY KEY (id, course_phase_id)
);

CREATE TABLE tutor (
    course_phase_id         uuid NOT NULL,
    course_participation_id uuid NOT NULL,
    university_login        text,
    first_name              text NOT NULL,
    last_name               text NOT NULL,
    team_id                 uuid NOT NULL,
    PRIMARY KEY (course_phase_id, course_participation_id),
    FOREIGN KEY (team_id, course_phase_id) REFERENCES team (id, course_phase_id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX tutor_phase_login_uk ON tutor (course_phase_id, university_login)
    WHERE university_login IS NOT NULL;
`

func TestNewPgxResolverRejectsBadConfiguration(t *testing.T) {
	t.Run("nil pool panics", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatalf("expected panic on nil pool")
			}
		}()
		NewPgxResolver(nil)
	})

	t.Run("empty table panics", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatalf("expected panic on empty table")
			}
		}()
		NewPgxResolver(&pgxpool.Pool{}, WithTable(""))
	})

	// The table name is interpolated, so it must be sanitized as an identifier.
	t.Run("table name is quoted", func(t *testing.T) {
		resolver, ok := NewPgxResolver(&pgxpool.Pool{}, WithTable(`tutor"; DROP TABLE team; --`)).(*pgxResolver)
		if !ok {
			t.Fatalf("expected a *pgxResolver")
		}
		if !strings.Contains(resolver.query, `"tutor""; DROP TABLE team; --"`) {
			t.Fatalf("table name was not sanitized: %s", resolver.query)
		}
	})
}

// An empty login must not reach the database, where it would match a row stored as
// the empty string. A zero-value pool would panic if the query ran.
func TestResolveTutorTeamShortCircuitsEmptyLogin(t *testing.T) {
	resolver := NewPgxResolver(&pgxpool.Pool{})
	for _, login := range []string{"", "   "} {
		if _, err := resolver.ResolveTutorTeam(context.Background(), uuid.New(), login); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("login %q: expected pgx.ErrNoRows, got %v", login, err)
		}
	}
}

func TestResolveTutorTeamAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := startPostgres(ctx, t)
	defer cleanup()

	if _, err := pool.Exec(ctx, schemaDDL); err != nil {
		t.Fatalf("could not create the schema: %v", err)
	}

	var (
		phase      = uuid.New()
		otherPhase = uuid.New()
		teamAlpha  = uuid.New()
		teamBeta   = uuid.New()
	)
	seed := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO team (id, course_phase_id, name) VALUES ($1, $2, 'Alpha')`, []any{teamAlpha, phase}},
		{`INSERT INTO team (id, course_phase_id, name) VALUES ($1, $2, 'Beta')`, []any{teamBeta, otherPhase}},
		{`INSERT INTO tutor (course_phase_id, course_participation_id, university_login, first_name, last_name, team_id)
		  VALUES ($1, $2, 'ab12cde', 'Ada', 'Lovelace', $3)`, []any{phase, uuid.New(), teamAlpha}},
		{`INSERT INTO tutor (course_phase_id, course_participation_id, university_login, first_name, last_name, team_id)
		  VALUES ($1, $2, NULL, 'No', 'Login', $3)`, []any{phase, uuid.New(), teamAlpha}},
		{`INSERT INTO tutor (course_phase_id, course_participation_id, university_login, first_name, last_name, team_id)
		  VALUES ($1, $2, 'gh34ijk', 'Grace', 'Hopper', $3)`, []any{otherPhase, uuid.New(), teamBeta}},
	}
	for _, s := range seed {
		if _, err := pool.Exec(ctx, s.query, s.args...); err != nil {
			t.Fatalf("could not seed: %v", err)
		}
	}

	resolver := NewPgxResolver(pool)

	t.Run("resolves the assigned team", func(t *testing.T) {
		teamID, err := resolver.ResolveTutorTeam(ctx, phase, "ab12cde")
		if err != nil || teamID != teamAlpha {
			t.Fatalf("expected %v, got %v (err=%v)", teamAlpha, teamID, err)
		}
	})

	t.Run("normalizes the login", func(t *testing.T) {
		teamID, err := resolver.ResolveTutorTeam(ctx, phase, "  AB12CDE ")
		if err != nil || teamID != teamAlpha {
			t.Fatalf("expected %v, got %v (err=%v)", teamAlpha, teamID, err)
		}
	})

	// A tutor of another phase must not resolve here, or scoping would leak across phases.
	t.Run("is scoped to the course phase", func(t *testing.T) {
		if _, err := resolver.ResolveTutorTeam(ctx, phase, "gh34ijk"); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("expected pgx.ErrNoRows, got %v", err)
		}
	})

	t.Run("unknown login is not a tutor", func(t *testing.T) {
		if _, err := resolver.ResolveTutorTeam(ctx, phase, "zz99zzz"); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("expected pgx.ErrNoRows, got %v", err)
		}
	})

	t.Run("reads a custom table", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `CREATE TABLE phase_tutor AS TABLE tutor`); err != nil {
			t.Fatalf("could not create the custom table: %v", err)
		}
		custom := NewPgxResolver(pool, WithTable("phase_tutor"))
		teamID, err := custom.ResolveTutorTeam(ctx, phase, "ab12cde")
		if err != nil || teamID != teamAlpha {
			t.Fatalf("expected %v, got %v (err=%v)", teamAlpha, teamID, err)
		}
	})
}

// startPostgres brings up a throwaway Postgres. It skips rather than fails when no
// container runtime is reachable, so the unit tests above still run locally.
func startPostgres(ctx context.Context, t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
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
			).WithDeadline(2 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Skipf("no container runtime available: %v", err)
	}

	terminate := func() { _ = container.Terminate(ctx) }

	host, err := container.Host(ctx)
	if err != nil {
		terminate()
		t.Fatalf("could not get the container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		terminate()
		t.Fatalf("could not get the container port: %v", err)
	}

	pool, err := pgxpool.New(ctx, fmt.Sprintf(
		"postgres://testuser:testpass@%s:%s/prompt?sslmode=disable", host, port.Port()))
	if err != nil {
		terminate()
		t.Fatalf("could not connect: %v", err)
	}

	return pool, func() {
		pool.Close()
		terminate()
	}
}
