package keycloakCoreRequests

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"

	"github.com/google/uuid"
	"github.com/prompt-edu/prompt-sdk/keycloakTokenVerifier/keycloakTokenVerifierDTO"
	log "github.com/sirupsen/logrus"
)

// ErrNotStudentOfCourse is returned when core denies course-phase access (401/403).
// Callers must compare with errors.Is rather than matching the message string.
var ErrNotStudentOfCourse = errors.New("not student of course")

// SendIsStudentRequest asks core whether the bearer of authHeader is a student
// of the given course phase. It returns ErrNotStudentOfCourse when core denies
// access (401/403), a descriptive error for any other non-200 status, and the
// decoded participation on 200.
func SendIsStudentRequest(coreURL url.URL, authHeader string, coursePhaseID uuid.UUID) (keycloakTokenVerifierDTO.GetCoursePhaseParticipation, error) {
	path := path.Join("/api/auth/course_phase", coursePhaseID.String(), "is_student")

	resp, err := sendRequest(coreURL, "GET", path, authHeader, nil)
	if err != nil {
		return keycloakTokenVerifierDTO.GetCoursePhaseParticipation{}, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Error("failed to close response body:", closeErr)
		}
	}()

	// Core denies cross-course/cross-phase access with 403 (and 401 if unauthenticated).
	// Both mean "not a student of this course phase" and must fail closed.
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		log.Info("Not student of course")
		return keycloakTokenVerifierDTO.GetCoursePhaseParticipation{IsStudentOfCoursePhase: false}, ErrNotStudentOfCourse
	}

	if resp.StatusCode != http.StatusOK {
		log.Error("Received non-OK response:", resp.Status)
		return keycloakTokenVerifierDTO.GetCoursePhaseParticipation{}, fmt.Errorf("unexpected core response: %s", resp.Status)
	}

	var isStudentResponse keycloakTokenVerifierDTO.GetCoursePhaseParticipation
	if err = json.NewDecoder(resp.Body).Decode(&isStudentResponse); err != nil {
		log.Error("Error decoding response body:", err)
		return keycloakTokenVerifierDTO.GetCoursePhaseParticipation{}, err
	}

	return isStudentResponse, nil
}
