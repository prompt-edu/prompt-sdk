package tutorscope

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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
// Keep the unique index: a login stored twice for one phase fails the lookup with
// an error rather than picking one of the teams.
// Store NULL, never the empty string, for a tutor without a login. The partial
// index does not exclude the empty string, so two login-less tutors of one phase
// would collide, and the resolver never matches an empty login anyway.
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
		`SELECT (SELECT team_id FROM %[1]s WHERE course_phase_id = $1 AND university_login = $2),
		        EXISTS (SELECT 1 FROM %[1]s WHERE course_phase_id = $1)`,
		pgx.Identifier{resolver.table}.Sanitize(),
	)
	return resolver
}

// ResolveTutorTeam returns the team the tutor is assigned to in this course phase.
// When the login belongs to no tutor it returns ErrNotATutor if the phase has
// tutors and pgx.ErrNoRows if it has none. The middleware tells the two apart, so
// neither may be replaced by another error.
func (r *pgxResolver) ResolveTutorTeam(ctx context.Context, coursePhaseID uuid.UUID, universityLogin string) (uuid.UUID, error) {
	// NULL matches no row, so an empty login cannot match a row stored as the empty string.
	var loginParam *string
	if login := NormalizeLogin(universityLogin); login != "" {
		loginParam = &login
	}

	var teamID pgtype.UUID
	var phaseHasTutors bool
	if err := r.pool.QueryRow(ctx, r.query, coursePhaseID, loginParam).Scan(&teamID, &phaseHasTutors); err != nil {
		return uuid.Nil, err
	}
	if teamID.Valid {
		return teamID.Bytes, nil
	}
	if phaseHasTutors {
		return uuid.Nil, ErrNotATutor
	}
	return uuid.Nil, pgx.ErrNoRows
}

type pgxTeamsResolver struct {
	pool  *pgxpool.Pool
	table string
	query string
}

// NewPgxTeamsResolver resolves every team of a tutor straight from a table, for
// services whose tutors can be assigned to several teams of a course phase. Pass it
// to TeamsMiddleware.
//
// It reads the same three columns as NewPgxResolver (course_phase_id,
// university_login, team_id), so it works on the canonical one-row-per-tutor table
// as well as on a table or view with one row per tutor and team. For the latter the
// canonical uniqueness is per team:
//
//	CREATE UNIQUE INDEX tutor_phase_login_team_uk
//	    ON tutor (course_phase_id, university_login, team_id)
//	    WHERE university_login IS NOT NULL;
//
// Teams are returned in a stable order. The storage rules of NewPgxResolver apply:
// store logins as NormalizeLogin returns them and NULL, never the empty string, for
// a tutor without a login.
func NewPgxTeamsResolver(pool *pgxpool.Pool, opts ...ResolverOption) TeamsResolver {
	if pool == nil {
		panic("tutorscope.NewPgxTeamsResolver: pool must not be nil")
	}

	// Reuse the option type: options only set the table.
	config := &pgxResolver{table: DefaultTable}
	for _, opt := range opts {
		opt(config)
	}
	if config.table == "" {
		panic("tutorscope.NewPgxTeamsResolver: table must not be empty")
	}

	resolver := &pgxTeamsResolver{pool: pool, table: config.table}
	resolver.query = fmt.Sprintf(
		`SELECT ARRAY(SELECT DISTINCT team_id FROM %[1]s
		              WHERE course_phase_id = $1 AND university_login = $2
		              ORDER BY team_id),
		        EXISTS (SELECT 1 FROM %[1]s WHERE course_phase_id = $1)`,
		pgx.Identifier{resolver.table}.Sanitize(),
	)
	return resolver
}

// ResolveTutorTeams returns every team the tutor is assigned to in this course
// phase. When the login belongs to no tutor it returns ErrNotATutor if the phase has
// tutors and pgx.ErrNoRows if it has none, like ResolveTutorTeam.
func (r *pgxTeamsResolver) ResolveTutorTeams(ctx context.Context, coursePhaseID uuid.UUID, universityLogin string) ([]uuid.UUID, error) {
	// NULL matches no row, so an empty login cannot match a row stored as the empty string.
	var loginParam *string
	if login := NormalizeLogin(universityLogin); login != "" {
		loginParam = &login
	}

	var teamIDs []uuid.UUID
	var phaseHasTutors bool
	if err := r.pool.QueryRow(ctx, r.query, coursePhaseID, loginParam).Scan(&teamIDs, &phaseHasTutors); err != nil {
		return nil, err
	}
	if len(teamIDs) > 0 {
		return teamIDs, nil
	}
	if phaseHasTutors {
		return nil, ErrNotATutor
	}
	return nil, pgx.ErrNoRows
}
