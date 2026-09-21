package keycloakTokenVerifier

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prompt-edu/prompt-sdk/keycloakTokenVerifier/keycloakCoreRequests"
	log "github.com/sirupsen/logrus"
)

// CourseRolesResolvedKey is the gin context key marking that the course-level
// Lecturer and Editor roles were looked up for this request. AuthenticationMiddleware
// admits a caller holding the global PROMPT_Lecturer role before that lookup runs, so
// without the marker IsLecturer and IsEditor being false is indistinguishable from the
// caller actually holding neither.
const CourseRolesResolvedKey = "courseRolesResolved"

// CourseRolesResolved reports whether TokenUser.IsLecturer and IsEditor reflect an
// actual lookup for this course phase rather than their zero values.
func CourseRolesResolved(c *gin.Context) bool {
	resolved, exists := c.Get(CourseRolesResolvedKey)
	if !exists {
		return false
	}
	ran, ok := resolved.(bool)
	return ok && ran
}

// Important: This requires a CoursePhaseID as a parameter.
func getLecturerAndEditorRole() gin.HandlerFunc {
	return func(c *gin.Context) {
		coursePhaseID, err := uuid.Parse(c.Param("coursePhaseID"))
		if err != nil {
			log.Error("Error parsing coursePhaseID:", err)
			_ = c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		if coursePhaseID == uuid.Nil {
			log.Error("Invalid coursePhaseID")
			_ = c.AbortWithError(http.StatusBadRequest, errors.New("coursePhaseID missing"))
			return
		}

		// TODO: Wrap this around a caching component
		// retrieve the relevant roles from the core
		tokenMapping, err := keycloakCoreRequests.SendCoursePhaseRoleMappingRequest(KeycloakTokenVerifierSingleton.CoreURL, c.GetHeader("Authorization"), coursePhaseID)
		if err != nil {
			if errors.Is(err, keycloakCoreRequests.ErrUnauthenticated) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "could not authenticate"})
				return
			}
			log.Error("Error getting course roles:", err)
			_ = c.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		// get roles from the context
		tokenUser, ok := GetTokenUser(c)
		if !ok {
			log.Error("Error getting token student")
			_ = c.AbortWithError(http.StatusInternalServerError, ErrUserNotInContext)
			return
		}
		userRoles := tokenUser.Roles

		// filter out the roles relevant for the current course phase
		isLecturer := userRoles[tokenMapping.CourseLecturerRole]
		isEditor := userRoles[tokenMapping.CourseEditorRole]

		// DEPRECATED: Keep this for backwards compatibility
		c.Set("isLecturer", isLecturer)
		c.Set("isEditor", isEditor)
		c.Set("customRolePrefix", tokenMapping.CustomRolePrefix)

		tokenUser.IsLecturer = isLecturer
		tokenUser.IsEditor = isEditor
		tokenUser.CustomRolePrefix = tokenMapping.CustomRolePrefix
		SetTokenUser(c, tokenUser)
		c.Set(CourseRolesResolvedKey, true)
	}
}
