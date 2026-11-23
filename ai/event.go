package ai

import (
	"errors"

	"github.com/google/uuid"
)

// The oversight events a phase service reports on a call it made: its UI showed the output, or
// the user accepted, edited or rejected it.
const (
	EventShown    = "shown"
	EventAccepted = "accepted"
	EventEdited   = "edited"
	EventRejected = "rejected"
)

var ErrInvalidEvent = errors.New("an event is shown, accepted, edited or rejected, and an edit needs a non-negative edit distance")

// EventRequest is the body of POST /ai/api/course_phase/:coursePhaseID/calls/:callID/events.
type EventRequest struct {
	Type string `json:"type" binding:"required,oneof=shown accepted edited rejected"`
	// EditDistance is required for an edit: how far the applied text is from the output.
	EditDistance *int `json:"editDistance,omitempty" binding:"omitempty,min=0"`
	// TargetID names what the phase applied the output to, for example an action item.
	TargetID *uuid.UUID `json:"targetId,omitempty"`
}

// Validate checks the event as the AI server will.
func (e EventRequest) Validate() error {
	switch e.Type {
	case EventShown, EventAccepted, EventEdited, EventRejected:
	default:
		return ErrInvalidEvent
	}
	if e.EditDistance != nil && *e.EditDistance < 0 {
		return ErrInvalidEvent
	}
	if e.Type == EventEdited && e.EditDistance == nil {
		return ErrInvalidEvent
	}
	return nil
}
