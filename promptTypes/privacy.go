package promptTypes

import (
	"github.com/google/uuid"
)

type StudentIdentifyingRequest struct {
  StudentID                   uuid.UUID   `json:"studentId" binding:"required"`
  CourseParticipationIDs      []uuid.UUID `json:"courseParticipationIds" binding:"required"`
  CoursePhaseParticipationIDs []uuid.UUID `json:"coursePhaseParticipationIds" binding:"required"`
}

const (
	PrivacyRouteStudentDataExport   string = "/privacy/student-data-export"
	PrivacyRouteStudentDataDeletion string = "/privacy/student-data-deletion"
)
