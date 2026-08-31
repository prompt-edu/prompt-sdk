package keycloakCoreRequests

import "github.com/google/uuid"

// SubjectIdentifiers contains all identifiers needed to locate a specific person's
// data across microservices. The core server populates and forwards these identifiers
// so that each microservice can scope its export or deletion to exactly one subject
// without performing additional lookups.
//
// There are three kinds of subjects:
//
//   - Student with a Keycloak account: all fields are populated. StudentID and
//     CourseParticipationIDs should be used to scope data access; UserID can be
//     used for any user-account-scoped data (e.g. instructor-note authorship).
//
//   - Student without a Keycloak account: UserID is uuid.Nil. StudentID and
//     CourseParticipationIDs are populated and should be used to scope data access.
//     Skip any user-account-scoped operations.
//
//   - Platform user: a person with a platform role such as lecturer, course editor,
//     or administrator who has no student record. Only UserID is set; StudentID is
//     uuid.Nil. Microservices should check StudentID == uuid.Nil to detect this case
//     and limit their scope to user-level data only.
//
// At least one of UserID and StudentID is always set; both being uuid.Nil is invalid.
type SubjectIdentifiers struct {
	// UserID is the platform-wide unique identifier of the user account.
	// uuid.Nil indicates a student without a Keycloak account; downstream services
	// should skip user-account-scoped operations in that case.
	UserID uuid.UUID `json:"userID"`

	// StudentID is the unique identifier of the student record.
	// Only set for student subjects — uuid.Nil indicates a platform user with no student record.
	StudentID uuid.UUID `json:"studentID"`

	// CourseParticipationIDs lists the IDs of all course participations belonging to the student.
	// Only populated for student subjects — empty for platform users.
	CourseParticipationIDs []uuid.UUID `json:"courseParticipationIDs"`
}
