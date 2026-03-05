package keycloakCoreRequests

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"path"

	"github.com/google/uuid"
	"github.com/ls1intum/prompt-sdk/keycloakTokenVerifier/keycloakTokenVerifierDTO"
	log "github.com/sirupsen/logrus"
)

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

	if resp.StatusCode == http.StatusUnauthorized {
		log.Info("Not student of course")
		return keycloakTokenVerifierDTO.GetCoursePhaseParticipation{IsStudentOfCoursePhase: false}, errors.New("not student of course")
	}

	if resp.StatusCode != http.StatusOK {
		log.Error("Received non-OK response:", resp.Status)
		return keycloakTokenVerifierDTO.GetCoursePhaseParticipation{}, nil
	}

	var isStudentResponse keycloakTokenVerifierDTO.GetCoursePhaseParticipation
	if err = json.NewDecoder(resp.Body).Decode(&isStudentResponse); err != nil {
		log.Error("Error decoding response body:", err)
		return keycloakTokenVerifierDTO.GetCoursePhaseParticipation{}, err
	}

	return isStudentResponse, nil
}
