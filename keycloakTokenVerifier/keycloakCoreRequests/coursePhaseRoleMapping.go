package keycloakCoreRequests

import (
	"encoding/json"
	"net/http"
	"net/url"
	"path"

	"github.com/google/uuid"
	"github.com/ls1intum/prompt-sdk/keycloakTokenVerifier/keycloakTokenVerifierDTO"
	log "github.com/sirupsen/logrus"
)

func SendCoursePhaseRoleMappingRequest(coreURL url.URL, authHeader string, coursePhaseID uuid.UUID) (keycloakTokenVerifierDTO.GetCourseRoles, error) {
	path := path.Join("/api/auth/course_phase", coursePhaseID.String(), "roles")

	resp, err := sendRequest(coreURL, "GET", path, authHeader, nil)
	if err != nil {
		return keycloakTokenVerifierDTO.GetCourseRoles{}, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Error("failed to close response body:", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		log.Error("Received non-OK response:", resp.Status)
		return keycloakTokenVerifierDTO.GetCourseRoles{}, nil
	}

	var authResponse keycloakTokenVerifierDTO.GetCourseRoles
	if err = json.NewDecoder(resp.Body).Decode(&authResponse); err != nil {
		log.Error("Error decoding response body:", err)
		return keycloakTokenVerifierDTO.GetCourseRoles{}, err
	}

	return authResponse, nil
}
