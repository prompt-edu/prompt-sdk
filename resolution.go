package promptSDK

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/prompt-edu/prompt-sdk/promptTypes"
	log "github.com/sirupsen/logrus"
)

type Resolution struct {
	DtoName       string    `json:"dtoName" binding:"required"`
	BaseURL       string    `json:"baseURL" binding:"required,url"`
	EndpointPath  string    `json:"endpointPath" binding:"required"`
	CoursePhaseID uuid.UUID `json:"coursePhaseID" binding:"required"`
}

type CoursePhaseParticipationsWithResolutions struct {
	Participations []promptTypes.CoursePhaseParticipationWithStudent `json:"participations"`
	Resolutions    []Resolution                                      `json:"resolutions" binding:"dive"`
}

type CoursePhaseParticipationWithResolutions struct {
	Participation promptTypes.CoursePhaseParticipationWithStudent `json:"participation"`
	Resolutions   []Resolution                                    `json:"resolutions"`
}

type PrevCoursePhaseData struct {
	PrevData    promptTypes.MetaData `json:"prevData"`
	Resolutions []Resolution         `json:"resolutions" binding:"dive"`
}

// buildURL constructs the request URL for a given resolution.
// extraPaths (such as a courseParticipationID) can be appended.
func buildURL(resolution Resolution, extraPaths ...string) (string, error) {
	base, err := validateResolutionTarget(resolution)
	if err != nil {
		return "", err
	}

	allPaths := append([]string{
		"course_phase",
		resolution.CoursePhaseID.String(),
		getEndpointPath(resolution.EndpointPath),
	}, extraPaths...)

	for _, path := range allPaths {
		if hasTraversalSegment(path) {
			return "", fmt.Errorf("resolution path %q must not contain traversal segments", path)
		}
	}

	return base.JoinPath(allPaths...).String(), nil
}

// validateResolutionTarget rejects resolution targets the caller's bearer token
// must not be forwarded to. The binding tags on Resolution do not cover this:
// resolutions are decoded with json.Unmarshal, which runs no validator.
func validateResolutionTarget(resolution Resolution) (*url.URL, error) {
	u, err := url.Parse(resolution.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid resolution baseURL %q: %w", resolution.BaseURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("resolution baseURL %q must use http or https", resolution.BaseURL)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("resolution baseURL %q has no host", resolution.BaseURL)
	}
	if u.User != nil {
		return nil, fmt.Errorf("resolution baseURL %q must not embed credentials", resolution.BaseURL)
	}
	return u, nil
}

// hasTraversalSegment reports whether a path holds a ".." segment, which
// url.JoinPath would resolve, moving the request off the intended path.
// Separators are decoded first, so "..%2f.." and "%2e%2e" are caught too.
func hasTraversalSegment(path string) bool {
	decoded, err := url.PathUnescape(path)
	if err != nil {
		return true
	}
	return slices.Contains(strings.Split(decoded, "/"), "..")
}

// parseAndValidate unmarshals the data into a map and ensures the expected key exists.
func parseAndValidate(data []byte, dtoName string) (interface{}, error) {
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	value, ok := result[dtoName]
	if !ok {
		log.Error("Failed to find expected key in response: ", dtoName)
		return nil, fmt.Errorf("failed to find expected key in response: %s", dtoName)
	}
	return value, nil
}

// ResolveParticipation resolves data for a single course participation.
func ResolveParticipation(authHeader string, resolution Resolution, courseParticipationID uuid.UUID) (interface{}, error) {
	url, err := buildURL(resolution, courseParticipationID.String())
	if err != nil {
		return nil, err
	}
	data, err := FetchJSON(url, authHeader)
	if err != nil {
		return nil, err
	}

	return parseAndValidate(data, resolution.DtoName)
}

// ResolveCoursePhaseData resolves data for a course phase.
func ResolveCoursePhaseData(authHeader string, resolution Resolution) (interface{}, error) {
	url, err := buildURL(resolution)
	if err != nil {
		return nil, err
	}
	data, err := FetchJSON(url, authHeader)
	if err != nil {
		return nil, err
	}

	return parseAndValidate(data, resolution.DtoName)
}

// ResolveAllParticipations resolves data for all participations and returns a map keyed by courseParticipationID.
func ResolveAllParticipations(authHeader string, resolution Resolution) (map[uuid.UUID]interface{}, error) {
	url, err := buildURL(resolution)
	if err != nil {
		return nil, err
	}
	data, err := FetchJSON(url, authHeader)
	if err != nil {
		return nil, err
	}

	var results []map[string]interface{}
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, err
	}

	formattedResults := make(map[uuid.UUID]interface{})
	for _, item := range results {
		idStr, ok := item["courseParticipationID"].(string)
		if !ok {
			log.Error("Failed to cast courseParticipationID to string")
			return nil, fmt.Errorf("failed to cast courseParticipationID to string")
		}
		participationID, err := uuid.Parse(idStr)
		if err != nil {
			log.Error("Failed to parse courseParticipationID: ", err)
			return nil, fmt.Errorf("failed to parse courseParticipationID: %v", err)
		}
		formattedResults[participationID] = item[resolution.DtoName]
	}

	return formattedResults, nil
}

// FetchAndMergeParticipationsWithResolutions fetches participations and enriches each with resolved data.
func FetchAndMergeParticipationsWithResolutions(coreURL string, authHeader string, coursePhaseID uuid.UUID) ([]promptTypes.CoursePhaseParticipationWithStudent, error) {
	url, err := url.JoinPath(coreURL, "api/course_phases", coursePhaseID.String(), "participations")
	if err != nil {
		return nil, err
	}
	data, err := FetchJSON(url, authHeader)
	if err != nil {
		return nil, err
	}

	var cppWithRes CoursePhaseParticipationsWithResolutions
	if err := json.Unmarshal(data, &cppWithRes); err != nil {
		return nil, err
	}

	for _, res := range cppWithRes.Resolutions {
		resolvedData, err := ResolveAllParticipations(authHeader, res)
		if err != nil {
			return nil, err
		}

		for idx, participation := range cppWithRes.Participations {
			if data, exists := resolvedData[participation.CourseParticipationID]; exists {
				if participation.PrevData == nil {
					participation.PrevData = make(promptTypes.MetaData)
				}
				participation.PrevData[res.DtoName] = data
				cppWithRes.Participations[idx] = participation
			}
		}
	}

	return cppWithRes.Participations, nil
}

// FetchAndMergeCourseParticipationWithResolution fetches a course participation by its courseParticipationID and enriches it with resolved data.
func FetchAndMergeCourseParticipationWithResolution(coreURL string, authHeader string, coursePhaseID, courseParticipationID uuid.UUID) (promptTypes.CoursePhaseParticipationWithStudent, error) {
	url, err := url.JoinPath(coreURL, "api/course_phases", coursePhaseID.String(), "participations", courseParticipationID.String())
	if err != nil {
		return promptTypes.CoursePhaseParticipationWithStudent{}, err
	}
	data, err := FetchJSON(url, authHeader)
	if err != nil {
		return promptTypes.CoursePhaseParticipationWithStudent{}, err
	}

	var cppWithRes CoursePhaseParticipationWithResolutions
	if err := json.Unmarshal(data, &cppWithRes); err != nil {
		return promptTypes.CoursePhaseParticipationWithStudent{}, err
	}

	for _, res := range cppWithRes.Resolutions {
		resolvedData, err := ResolveParticipation(authHeader, res, courseParticipationID)
		if err != nil {
			return promptTypes.CoursePhaseParticipationWithStudent{}, err
		}

		participation := cppWithRes.Participation
		if resolvedData != nil {
			if participation.PrevData == nil {
				participation.PrevData = make(promptTypes.MetaData)
			}
			participation.PrevData[res.DtoName] = data
			cppWithRes.Participation = participation
		}
	}

	return cppWithRes.Participation, nil
}

func FetchAndMergeCoursePhaseWithResolution(coreURL string, authHeader string, coursePhaseID uuid.UUID) (promptTypes.MetaData, error) {
	url, err := url.JoinPath(coreURL, "api/course_phases", coursePhaseID.String(), "course_phase_data")
	if err != nil {
		return nil, err
	}
	data, err := FetchJSON(url, authHeader)
	if err != nil {
		return nil, err
	}

	var cpWithRes PrevCoursePhaseData
	if err := json.Unmarshal(data, &cpWithRes); err != nil {
		return nil, err
	}

	if cpWithRes.PrevData == nil {
		cpWithRes.PrevData = make(promptTypes.MetaData)
	}

	for _, res := range cpWithRes.Resolutions {
		resolvedData, err := ResolveCoursePhaseData(authHeader, res)
		if err != nil {
			return nil, err
		}
		cpWithRes.PrevData[res.DtoName] = resolvedData
	}
	return cpWithRes.PrevData, nil
}

// getEndpointPath trims leading and trailing slashes from the endpoint path.
func getEndpointPath(endpointPath string) string {
	return strings.Trim(endpointPath, "/")
}
