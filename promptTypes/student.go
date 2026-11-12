package promptTypes

import (
	"github.com/jackc/pgx/v5/pgtype"
)

// Student represents a student in the Prompt system with comprehensive academic and personal information.
// This type extends the base Person type with student-specific fields required for course management,
// academic tracking, and administrative purposes.
type Student struct {
	// Person contains the basic identity information (ID, FirstName, LastName, Email, UniversityLogin).
	Person

	// MatriculationNumber is the student's unique university identification number.
	// This number is assigned by the university and remains constant throughout the student's enrollment.
	MatriculationNumber string `json:"matriculationNumber" binding:"matriculationNumber"`

	// HasUniversityAccount indicates whether the student has been granted access to university systems.
	// This flag helps determine what services and resources the student can access.
	HasUniversityAccount bool `json:"hasUniversityAccount"`

	// Gender represents the student's gender identity for demographic and statistical purposes.
	// Must be one of: "male", "female", "diverse", or "prefer_not_to_say".
	Gender Gender `json:"gender" binding:"oneof=male female diverse prefer_not_to_say"`

	// Nationality represents the student's nationality or citizenship.
	// This information may be used for visa requirements, international student services, or statistics.
	Nationality string `json:"nationality"`

	// StudyDegree indicates the type of degree the student is pursuing.
	// Must be either "bachelor" or "master".
	StudyDegree StudyDegree `json:"studyDegree" binding:"oneof=bachelor master"`

	// StudyProgram is the specific program or major the student is enrolled in.
	// Examples: "Computer Science", "Information Systems", "Management and Technology".
	StudyProgram string `json:"studyProgram"`

	// CurrentSemester indicates which semester the student is currently in within their program.
	// This helps with course eligibility, academic planning, and progress tracking.
	CurrentSemester pgtype.Int4 `json:"currentSemester"`
}
