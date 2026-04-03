package keycloakCoreRequests

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	log "github.com/sirupsen/logrus"
)

// ErrUnauthorized is returned when the core service rejects the token with 401.
var ErrUnauthorized = errors.New("unauthorized")

func GetSubjectIdentifiers(coreURL url.URL, authHeader string) (SubjectIdentifiers, error) {
	resp, err := sendRequest(coreURL, "GET", "/api/auth/subject_identifiers", authHeader, nil)
	if err != nil {
		return SubjectIdentifiers{}, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Error("failed to close response body:", closeErr)
		}
	}()

	if resp.StatusCode == http.StatusUnauthorized {
		log.Info("Received 401 from core for subject identifiers request")
		return SubjectIdentifiers{}, ErrUnauthorized
	}

	if resp.StatusCode != http.StatusOK {
		log.Error("Received non-OK response:", resp.Status)
		return SubjectIdentifiers{}, fmt.Errorf("unexpected response from core: %s", resp.Status)
	}

	var subjectIdentifiers SubjectIdentifiers
	if err = json.NewDecoder(resp.Body).Decode(&subjectIdentifiers); err != nil {
		log.Error("Error decoding response body:", err)
		return SubjectIdentifiers{}, err
	}

	return subjectIdentifiers, nil
}
