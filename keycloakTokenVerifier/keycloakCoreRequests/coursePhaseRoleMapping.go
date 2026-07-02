package keycloakCoreRequests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"

	"github.com/google/uuid"
	"github.com/prompt-edu/prompt-sdk/keycloakTokenVerifier/keycloakTokenVerifierDTO"
	log "github.com/sirupsen/logrus"
)

// SendCoursePhaseRoleMappingRequest fetches the lecturer/editor/custom role
// mapping for the given course phase from core. It fails closed by returning an
// error on any non-200 status, so callers never receive an empty mapping that
// could weaken the custom-role check in the auth middleware.
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

	// Fail closed: an empty role mapping would leave CustomRolePrefix unset, causing
	// the custom-role check in the auth middleware to match un-prefixed roles.
	if resp.StatusCode != http.StatusOK {
		log.Error("Received non-OK response:", resp.Status)
		return keycloakTokenVerifierDTO.GetCourseRoles{}, fmt.Errorf("unexpected core response: %s", resp.Status)
	}

	var authResponse keycloakTokenVerifierDTO.GetCourseRoles
	if err = json.NewDecoder(resp.Body).Decode(&authResponse); err != nil {
		log.Error("Error decoding response body:", err)
		return keycloakTokenVerifierDTO.GetCourseRoles{}, err
	}

	return authResponse, nil
}
