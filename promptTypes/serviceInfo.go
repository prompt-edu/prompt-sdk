package promptTypes

import (
	"maps"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Capability key constants — use these when populating ServiceInfo.Capabilities
// to avoid typos and get IDE support.
const (
	// CapabilityPhaseCopy indicates support for copying course phase data
	// from a source phase to a target phase.
	// Expected endpoint: POST .../course_phase/:coursePhaseID/copy
	CapabilityPhaseCopy = "phase.copy"

	// CapabilityPhaseConfig indicates support for reporting the configuration
	// status of a course phase.
	// Expected endpoint: GET .../course_phase/:coursePhaseID/config
	CapabilityPhaseConfig = "phase.config"

	// CapabilityPrivacyExport indicates support for assembling and returning all
	// data associated with a subject for GDPR export purposes.
	// Expected endpoint: POST .../privacy/data-export
	CapabilityPrivacyExport = "privacy.export"

	// CapabilityPrivacyDeletion indicates support for deleting all data
	// associated with a subject on GDPR deletion request.
	// Expected endpoint: POST .../privacy/data-deletion
	CapabilityPrivacyDeletion = "privacy.deletion"
)

// ServiceInfo describes a course phase microservice — its identity, health, and capabilities.
// It is returned by the GET /api/info endpoint and used by the core system and admin
// dashboard to determine service status and available features.
//
// Populate Capabilities using the Capability* constants defined in this package.
// Any capability key that is absent is treated as false (not supported) by consumers.
type ServiceInfo struct {
	// ServiceName is the human-readable name of the microservice (e.g. "interview").
	ServiceName string `json:"serviceName"`

	// Version is the deployed version or image tag of the service. Optional.
	// Set this from the SERVER_IMAGE_TAG environment variable.
	Version string `json:"version,omitempty"`

	// Healthy reports whether the service is fully operational at the time of the request.
	// This should reflect live state (e.g. a database ping), not a static value.
	Healthy bool `json:"healthy"`

	// Capabilities is a map of capability keys to booleans indicating support.
	// Use the Capability* constants as keys. Absent keys are treated as false.
	Capabilities map[string]bool `json:"capabilities"`
}

// RegisterInfoEndpoint registers a public GET /info route on the given router group.
// The static fields of info are returned as-is; the Healthy field is determined
// dynamically by calling healthCheck on every request.
//
// Pass nil for healthCheck to always report healthy: true.
//
// Example registration (in a microservice's main.go):
//
//	baseApi := router.Group("interview/api")
//	promptTypes.RegisterInfoEndpoint(baseApi, promptTypes.ServiceInfo{
//	    ServiceName: "interview",
//	    Version:     promptSDK.GetEnv("SERVER_IMAGE_TAG", ""),
//	    Capabilities: map[string]bool{
//	        promptTypes.CapabilityPhaseCopy:   true,
//	        promptTypes.CapabilityPhaseConfig: true,
//	    },
//	}, func() bool {
//	    return conn.Ping(context.Background()) == nil
//	})
func RegisterInfoEndpoint(router *gin.RouterGroup, info ServiceInfo, healthCheck func() bool) {
	caps := make(map[string]bool, len(info.Capabilities))
	maps.Copy(caps, info.Capabilities)
	router.GET("/info", func(c *gin.Context) {
		healthy := true
		if healthCheck != nil {
			healthy = healthCheck()
		}
		c.JSON(http.StatusOK, ServiceInfo{
			ServiceName:  info.ServiceName,
			Version:      info.Version,
			Healthy:      healthy,
			Capabilities: caps,
		})
	})
}
