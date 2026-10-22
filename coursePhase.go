package promptSDK

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"github.com/prompt-edu/prompt-sdk/promptTypes"
	"github.com/prompt-edu/prompt-sdk/utils"
)

// FetchCoursePhase reads a course phase from core with the caller's token, so core decides whether
// the caller may see it. Any answer other than 200 is an error.
func FetchCoursePhase(ctx context.Context, coreURL, authHeader string, coursePhaseID uuid.UUID) (promptTypes.CoursePhase, error) {
	phaseURL, err := url.JoinPath(coreURL, "api/course_phases", coursePhaseID.String())
	if err != nil {
		return promptTypes.CoursePhase{}, fmt.Errorf("build course phase url: %w", err)
	}
	resp, err := utils.SendCoreRequest(ctx, http.MethodGet, authHeader, nil, phaseURL)
	if err != nil {
		return promptTypes.CoursePhase{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return promptTypes.CoursePhase{}, fmt.Errorf("core answered %d for course phase %s", resp.StatusCode, coursePhaseID)
	}
	var coursePhase promptTypes.CoursePhase
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&coursePhase); err != nil {
		return promptTypes.CoursePhase{}, fmt.Errorf("decode course phase %s: %w", coursePhaseID, err)
	}
	return coursePhase, nil
}
