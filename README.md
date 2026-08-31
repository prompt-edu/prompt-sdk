# Prompt SDK (Go)

A lightweight Go SDK for building Prompt services.

## What it provides

- Keycloak-based authentication middleware for Gin with role-aware access control
- Cross-phase tutor team scoping: one read gate, one write policy, one tutor table shape
- Standardized HTTP endpoints for course phase config/copy flows
- Resolution helpers to fetch and merge data from the Prompt Core and other services
- Shared domain models used across Prompt services
- Utilities: CORS, environment variables, DB rollback helper, JSON fetching, and custom validators

## Installation

Install via your Go module tooling (module path: github.com/prompt-edu/prompt-sdk). Go 1.26+ is required.

## Usage overview (high level)

1. Initialize authentication once at startup by providing Keycloak base URL, realm, and the Prompt Core base URL.

2. Protect Gin routes with the provided role-aware middleware. For course-phase roles, routes must include the path parameter `:coursePhaseID`.

3. Read the authenticated user from the Gin context; the SDK attaches a token-derived user struct with roles and per-course-phase information.

4. Use standardized endpoints from `promptTypes` to expose consistent module behavior (e.g., a GET config endpoint and a POST copy endpoint).

5. Use resolution helpers to fetch external data and merge it into your responses. Remote services are expected to return JSON that contains a named key corresponding to the expected DTO name.

## Authentication and roles

- Global roles (from Keycloak token): "PROMPT_Admin", "PROMPT_Lecturer"
- Course-phase roles (resolved via Core using `:coursePhaseID`): "Lecturer", "Editor", "Student"
- Custom roles supported via a prefix provided by Core; any additional role names can be checked against that prefix
- The middleware verifies standard OIDC fields and attaches a token user to the request context

## Tutor team scoping

A tutor is a course editor recorded as responsible for exactly one team of a course phase. The
`tutorscope` package owns that access control for every phase service that models teams, so all of
them behave the same way. A module that consumes a team with tutor input does not have to know which
phase produced it.

Reads and writes are deliberately asymmetric:

- `tutorscope.Middleware(resolver)` resolves the tutor's team onto the request and fails **open**.
  An editor with no resolvable tutor record keeps full read access. Handlers narrow their responses
  with `tutorscope.TeamID(c)`.
- `tutorscope.AuthorizeWrite(c)` resolves the same request into a write scope and fails **closed**.
  Admins and course lecturers write any team; an editor with a resolved tutor team is confined to it;
  everyone else is denied. Reusing the read gate for writes would hand every unresolvable editor
  write access to every team of the phase.

Apply a confined scope inside the mutating statement rather than reading the current team first, so
authorization and mutation are atomic:

```go
access, err := tutorscope.AuthorizeWrite(c)   // ErrWriteDenied -> 403, ErrScopingNotApplied -> 500
if err == nil && !access.AllowsTeam(targetTeam) {
    err = tutorscope.ErrWriteDenied           // a tutor may not write into a team that is not theirs
}
rows, err := queries.MoveParticipant(ctx, MoveParticipantParams{
    TeamID:         targetTeam,
    ExpectedTeamID: access.Guard(),           // NULL when unrestricted
})
// rows == 0 for a confined caller means the row moved out of their team: ErrWriteDenied
```

Supply the tutor lookup either by implementing `tutorscope.Resolver` over your own queries, or with
`tutorscope.NewPgxResolver(pool)` against a `tutor` table with the canonical shape documented on that
constructor. Store logins as `tutorscope.NormalizeLogin` returns them.

## Resolution helpers

- Describe where to fetch supplemental data (base URL, endpoint path, course phase ID, expected DTO name)
- Resolve for a single participation, for all participations, or for the entire course phase
- Merge resolved data into metadata maps for consistent downstream usage

## Standard endpoints

- Config endpoint: uniform GET endpoint to report whether required configuration elements are present for a course phase
- Copy endpoint: uniform POST endpoint to copy internal state from one course phase to another

## Shared domain models

- Reusable types for people, students, teams, gender, study degrees, and generic metadata maps
- Intended as cross-service contracts to keep modules in sync

## Utilities and validation

- CORS middleware; environment helper; DB transaction rollback helper; simple JSON fetch helper
- Validation integrated with Gin: matriculation numbers and university logins (TUM ID format)

## Testing

Run your standard Go tests within the module (for example with your usual tooling).

## License

MIT © TUM Applied Education Technologies — see the LICENSE file.
