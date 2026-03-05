package keycloakTokenVerifier

import (
	"net/url"

	log "github.com/sirupsen/logrus"
)

type KeycloakTokenVerifier struct {
	KeycloakURL             url.URL
	Realm                   string
	ClientID                string
	expectedAuthorizedParty string
	CoreURL                 url.URL
}

var KeycloakTokenVerifierSingleton *KeycloakTokenVerifier

func InitKeycloakTokenVerifier(KeycloakURL, Realm, CoreURL string) error {
	// Parse the Keycloak URL
	keycloakURL, err := url.Parse(KeycloakURL)
	if err != nil {
		log.Error("Failed to parse Keycloak URL: ", err)
		return err
	}

	// Parse the Core URL
	coreURL, err := url.Parse(CoreURL)
	if err != nil {
		log.Error("Failed to parse Core URL: ", err)
		return err
	}

	KeycloakTokenVerifierSingleton = &KeycloakTokenVerifier{
		KeycloakURL:             *keycloakURL,
		Realm:                   Realm,
		ClientID:                "prompt-server",
		expectedAuthorizedParty: "prompt-client",
		CoreURL:                 *coreURL,
	}

	// init the middleware
	err = InitKeycloakVerifier()
	if err != nil {
		log.Error("Failed to initialize keycloak verifier: ", err)
		return err
	}
	return nil
}
