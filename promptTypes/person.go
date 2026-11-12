package promptTypes

import "github.com/google/uuid"

// Person represents an individual person in the Prompt system.
// This type is used to identify and store basic information about users,
// students, lecturers, and other participants within the platform.
type Person struct {
	// ID is the unique identifier for the person using UUID v4 format.
	// The uuid.UUID type ensures type safety and automatic validation during JSON unmarshaling.
	ID uuid.UUID `json:"id"`

	// FirstName stores the first name as provided during initial application or profile updates.
	FirstName string `json:"firstName"`

	// LastName stores the last name as provided during initial application or profile updates.
	LastName string `json:"lastName"`

	// Email is the person's contact email address. It is empty when the source does not provide it.
	Email string `json:"email" binding:"omitempty,email"`

	// UniversityLogin is the person's username for university systems and services.
	// It is empty when the source does not provide it.
	UniversityLogin string `json:"universityLogin" binding:"omitempty,universityLogin"`
}
