package keycloakTokenVerifier

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prompt-edu/prompt-sdk/keycloakTokenVerifier/keycloakCoreRequests"
	log "github.com/sirupsen/logrus"
)

// Important: This requires a CoursePhaseID as a parameter.
func isStudentOfCoursePhaseMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		coursePhaseID, err := uuid.Parse(c.Param("coursePhaseID"))
		if err != nil {
			log.Error("Error parsing coursePhaseID: ", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}

		if coursePhaseID == uuid.Nil {
			log.Error("Invalid coursePhaseID")
			_ = c.AbortWithError(http.StatusBadRequest, errors.New("coursePhaseID missing"))
			return
		}

		// TODO: Wrap this around a caching component
		// request from the core if the user is a student of the course phase
		isStudentResponse, err := keycloakCoreRequests.SendIsStudentRequest(KeycloakTokenVerifierSingleton.CoreURL, c.GetHeader("Authorization"), coursePhaseID)
		if err != nil {
			if errors.Is(err, keycloakCoreRequests.ErrNotStudentOfCourse) {
				// Core denied access (403/401): the caller is not a student of this
				// course phase. Fail closed on both the context keys and the token user.
				c.Set("isStudentOfCourse", false)
				c.Set("isStudentOfCoursePhase", false)

				if tokenUser, ok := GetTokenUser(c); ok {
					tokenUser.IsStudentOfCourse = false
					tokenUser.IsStudentOfCoursePhase = false
					SetTokenUser(c, tokenUser)
				}
			} else {
				log.Error("Error verifying course phase participation:", err)
				_ = c.AbortWithError(http.StatusInternalServerError, err)
				return
			}
		} else {
			// Reaching here means core returned 200, which it only does after
			// authorizing the caller as a student of this phase's course.
			c.Set("isStudentOfCourse", true)
			c.Set("isStudentOfCoursePhase", isStudentResponse.IsStudentOfCoursePhase)
			c.Set("courseParticipationID", isStudentResponse.CourseParticipationID)

			tokenUser, ok := GetTokenUser(c)
			if !ok {
				log.Error("Error getting token student")
				_ = c.AbortWithError(http.StatusInternalServerError, ErrUserNotInContext)
				return
			}
			tokenUser.IsStudentOfCourse = true
			tokenUser.IsStudentOfCoursePhase = isStudentResponse.IsStudentOfCoursePhase
			tokenUser.CourseParticipationID = isStudentResponse.CourseParticipationID
			SetTokenUser(c, tokenUser)
		}
	}
}
