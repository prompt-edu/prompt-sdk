package keycloakCoreRequests

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	log "github.com/sirupsen/logrus"
)

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
		log.Info("Not student of course")
		return SubjectIdentifiers{}, errors.New("not student of course")
	}

	if resp.StatusCode != http.StatusOK {
		log.Error("Received non-OK response:", resp.Status)
		return SubjectIdentifiers{}, nil
	}

	var subjectIdentifiers SubjectIdentifiers
	if err = json.NewDecoder(resp.Body).Decode(&subjectIdentifiers); err != nil {
		log.Error("Error decoding response body:", err)
		return SubjectIdentifiers{}, err
	}

	return subjectIdentifiers, nil
}
