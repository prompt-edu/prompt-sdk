// Package audit provides a unified, low-effort audit-logging API shared by the
// core service and every course phase microservice. Services register a single
// Gin middleware that automatically captures mutating requests; developers may
// additionally name events with Describe, emit rich or background events with
// Record, or opt out with Skip/Suppress. Only the Sink (how an entry is
// persisted) differs per service: core inserts into its own database, phases
// ship entries to core over HTTP.
package audit

import "context"

// Outcome values distinguish a successful action from a denied attempt.
const (
	OutcomeSuccess = "success"
	OutcomeDenied  = "denied"
)

// Event is the neutral, transport-agnostic audit record. The same struct is
// produced by the auto-capture middleware and by explicit Record calls, and is
// consumed by every Sink implementation.
type Event struct {
	// Actor — who performed the action. In PROMPT every action traces to a
	// human; background work carries the initiating human's identity.
	ActorID    string   `json:"actorID"`
	ActorName  string   `json:"actorName"`
	ActorEmail string   `json:"actorEmail"`
	ActorRoles []string `json:"actorRoles"`
	ActorRole  string   `json:"actorRole"`

	// What happened.
	Action     string `json:"action"`     // human-readable, e.g. "Created slot"
	ActionKey  string `json:"actionKey"`  // machine key, e.g. "POST /api/.../slots"
	Outcome    string `json:"outcome"`    // OutcomeSuccess | OutcomeDenied
	EntityType string `json:"entityType"` // optional
	EntityID   string `json:"entityID"`   // optional
	EntityName string `json:"entityName"` // snapshotted human-readable subject

	// Where.
	CourseID      string `json:"courseID"`      // optional; null => admin-only
	CoursePhaseID string `json:"coursePhaseID"` // optional
	SourceService string `json:"sourceService"` // "core", "interview", ...

	// HTTP context (empty for non-request events).
	HTTPMethod string `json:"httpMethod"`
	HTTPPath   string `json:"httpPath"`
	HTTPStatus int    `json:"httpStatus"`

	// Metadata holds identifiers and change summaries — never raw sensitive
	// payloads (grade values, note contents).
	Metadata map[string]any `json:"metadata"`
}

// Sink persists an audit Event. It is a small consumer-side interface: core
// provides a database-backed sink, phases provide an HTTP sink (NewCoreSink).
type Sink interface {
	Record(ctx context.Context, e Event) error
}
