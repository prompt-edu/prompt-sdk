package tutorscope

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultTable is the table NewPgxResolver reads unless WithTable overrides it.
const DefaultTable = "tutor"

// ResolverOption configures NewPgxResolver.
type ResolverOption func(*pgxResolver)

// WithTable reads tutor rows from a table other than DefaultTable. The name is
// sanitized as an SQL identifier; the column names remain the contract.
func WithTable(table string) ResolverOption {
	return func(r *pgxResolver) {
		r.table = table
	}
}

type pgxResolver struct {
	pool  *pgxpool.Pool
	table string
	query string
}

// NewPgxResolver resolves tutor teams straight from a table with the canonical
// shape, so a phase service needs neither an adapter type nor a generated query.
// A service that would rather keep the lookup in its own sqlc queries can pass its
// own Resolver to Middleware instead; the interface stays the extension point.
//
// The canonical schema is:
//
//	CREATE TABLE tutor (
//	    course_phase_id         uuid NOT NULL,
//	    course_participation_id uuid NOT NULL,
//	    university_login        text,
//	    first_name              text NOT NULL,
//	    last_name               text NOT NULL,
//	    team_id                 uuid NOT NULL,
//	    PRIMARY KEY (course_phase_id, course_participation_id),
//	    FOREIGN KEY (team_id, course_phase_id)
//	        REFERENCES team (id, course_phase_id) ON DELETE CASCADE
//	);
//
//	CREATE UNIQUE INDEX tutor_phase_login_uk ON tutor (course_phase_id, university_login)
//	    WHERE university_login IS NOT NULL;
//
// Only course_phase_id, university_login and team_id are read. Store logins as
// NormalizeLogin returns them: the lookup is an exact match so it can use the index.
func NewPgxResolver(pool *pgxpool.Pool, opts ...ResolverOption) Resolver {
	if pool == nil {
		panic("tutorscope.NewPgxResolver: pool must not be nil")
	}

	resolver := &pgxResolver{pool: pool, table: DefaultTable}
	for _, opt := range opts {
		opt(resolver)
	}
	if resolver.table == "" {
		panic("tutorscope.NewPgxResolver: table must not be empty")
	}

	resolver.query = fmt.Sprintf(
		`SELECT team_id FROM %s WHERE course_phase_id = $1 AND university_login = $2`,
		pgx.Identifier{resolver.table}.Sanitize(),
	)
	return resolver
}

// ResolveTutorTeam returns the team the tutor is assigned to in this course phase,
// or pgx.ErrNoRows when the login belongs to no tutor of the phase. The middleware
// reads that sentinel as "not a tutor", so it must not be wrapped.
func (r *pgxResolver) ResolveTutorTeam(ctx context.Context, coursePhaseID uuid.UUID, universityLogin string) (uuid.UUID, error) {
	login := NormalizeLogin(universityLogin)
	if login == "" {
		// An empty login would otherwise match a row stored as the empty string.
		return uuid.Nil, pgx.ErrNoRows
	}

	var teamID uuid.UUID
	if err := r.pool.QueryRow(ctx, r.query, coursePhaseID, login).Scan(&teamID); err != nil {
		return uuid.Nil, err
	}
	return teamID, nil
}
