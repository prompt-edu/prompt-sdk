package promptTypes

import (
	"github.com/google/uuid"
)

type StudentIdentifyingRequest struct {
	StudentID                   uuid.UUID   `json:"studentId"`
	CourseParticipationIDs      []uuid.UUID `json:"courseParticipationIds"`
	CoursePhaseParticipationIDs []uuid.UUID `json:"coursePhaseParticipationIds"`
}

const (
	PrivacyRouteStudentDataExport   string = "/privacy/student-data-export"
	PrivacyRouteStudentDataDeletion string = "/privacy/student-data-deletion"
)
