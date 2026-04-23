package keycloakCoreRequests

import "github.com/google/uuid"

// SubjectIdentifiers contains all identifiers needed to locate a specific person's
// data across microservices. The core server populates and forwards these identifiers
// so that each microservice can scope its export or deletion to exactly one subject
// without performing additional lookups.
//
// There are two kinds of subjects:
//
//   - Student: a person who participates in courses. All fields are populated —
//     StudentID and CourseParticipationIDs are
//     guaranteed to be non-empty and should be used to scope data access.
//
//   - Platform user: a person with a platform role such as lecturer, course editor,
//     or administrator who has no student record. Only UserID is guaranteed to be
//     set. Microservices should check StudentID == uuid.Nil to detect this case
//     and limit their scope to user-level data only.
type SubjectIdentifiers struct {
	// UserID is the platform-wide unique identifier of the user account.
	// Always present regardless of subject type.
	UserID uuid.UUID `json:"userID" binding:"required"`

	// StudentID is the unique identifier of the student record.
	// Only set for student subjects — uuid.Nil indicates a platform user with no student record.
	StudentID uuid.UUID `json:"studentID"`

	// CourseParticipationIDs lists the IDs of all course participations belonging to the student.
	// Only populated for student subjects — empty for platform users.
	CourseParticipationIDs []uuid.UUID `json:"courseParticipationIDs"`
}
