package promptSDK

import (
	"fmt"
	"strings"

	"github.com/prompt-edu/prompt-sdk/utils"
	log "github.com/sirupsen/logrus"
)

// InitPhaseKeycloak initializes the Keycloak authentication middleware using environment variables.
// It returns an error if the initialization fails instead of using log.Fatal, allowing callers to handle errors.
func InitPhaseKeycloak() error {
	baseURL := GetEnv("KEYCLOAK_HOST", "http://localhost:8081")
	if !strings.HasPrefix(baseURL, "http") {
		log.Warn("Keycloak host does not start with http(s). Adding https:// as prefix.")
		baseURL = "https://" + baseURL
	}

	realm := GetEnv("KEYCLOAK_REALM_NAME", "prompt")

	coreURL := utils.GetCoreUrl()
	err := InitAuthenticationMiddleware(baseURL, realm, coreURL)
	if err != nil {
		return fmt.Errorf("failed to initialize keycloak: %w", err)
	}
	return nil
}
