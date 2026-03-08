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
			if err.Error() == "not student of course" {
				c.Set("isStudentOfCourse", false)
				c.Set("isStudentOfCoursePhase", false)
			} else {
				log.Error("Error getting course roles:", err)
				_ = c.AbortWithError(http.StatusInternalServerError, err)
				return
			}
		} else {
			// DEPRECATED: Keep this for backwards compatibility
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
