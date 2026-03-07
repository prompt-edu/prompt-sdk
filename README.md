# Prompt SDK (Go)

A lightweight Go SDK for building Prompt services.

## What it provides

- Keycloak-based authentication middleware for Gin with role-aware access control
- Standardized HTTP endpoints for course phase config/copy flows
- Resolution helpers to fetch and merge data from the Prompt Core and other services
- Shared domain models used across Prompt services
- Utilities: CORS, environment variables, DB rollback helper, JSON fetching, and custom validators

## Installation

Install via your Go module tooling (module path: github.com/prompt-edu/prompt-sdk). Go 1.24+ is required.

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
