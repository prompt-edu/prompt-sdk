package promptTypes

import "github.com/google/uuid"

// CoursePhase is a course phase as core's GET /api/course_phases/:id describes it.
type CoursePhase struct {
	ID                  uuid.UUID `json:"id"`
	CourseID            uuid.UUID `json:"courseID"`
	Name                string    `json:"name"`
	IsInitialPhase      bool      `json:"isInitialPhase"`
	RestrictedData      MetaData  `json:"restrictedData"`
	StudentReadableData MetaData  `json:"studentReadableData"`
	CoursePhaseTypeID   uuid.UUID `json:"coursePhaseTypeID"`
	CoursePhaseTypeName string    `json:"coursePhaseTypeName"`
}
