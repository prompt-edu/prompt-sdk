package keycloakTokenVerifier

import (
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// AuthenticationMiddleware creates a composite middleware which always
// applies KeycloakMiddleware first and conditionally chains additional
// middlewares based on the allowed roles:
//   - If allowedRoles contains "Lecturer", "Editor" or a custom role name
//     (any value other than "Admin" or "Student"), then it calls GetLecturerAndEditorRole.
//     For custom roles the middleware checks if the user's roles include customRolePrefix+customRole.
//   - If allowedRoles contains "Student", then it calls IsStudentOfCoursePhaseMiddleware.
//
// Missing or invalid credentials abort with 401. An authenticated caller
// without any of the allowed roles aborts with 403.
func AuthenticationMiddleware(allowedRoles ...string) gin.HandlerFunc {
	authorize := authorizationMiddleware(allowedRoles...)
	return func(c *gin.Context) {
		// Always run Keycloak middleware first.
		KeycloakMiddleware()(c)
		if c.IsAborted() {
			return
		}
		authorize(c)
	}
}

// authorizationMiddleware checks the TokenUser set by KeycloakMiddleware against allowedRoles.
func authorizationMiddleware(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		allowedSet := buildAllowedRolesSet(allowedRoles)

		tokenUser, ok := GetTokenUser(c)
		if !ok {
			log.Error("Error getting token student")
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrUserNotInContext)
			return
		}
		userRoles := tokenUser.Roles

		// 1.) Directly grant access for PROMPT_Admin or PROMPT_Lecturer.
		if checkDirectRole(PromptAdmin, allowedSet, userRoles) ||
			checkDirectRole(PromptLecturer, allowedSet, userRoles) {
			c.Next()
			return
		}

		// This allows to use the middleware without coursePhaseID, if only PROMPT_Admin & PROMPT_Lecturer are allowed.
		if onlyContainsAdminAndLecturer(allowedSet) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			return
		}

		// 2.) Check for Lecturer, Editor, or custom group roles.
		if requiresLecturerOrCustom(allowedSet, allowedRoles) {
			getLecturerAndEditorRole()(c)
			if c.IsAborted() {
				return
			}

			tokenUser, ok = GetTokenUser(c)
			if !ok {
				log.Error("Error refreshing the token student")
				c.AbortWithStatusJSON(http.StatusUnauthorized, ErrUserNotInContext)
				return
			}

			if _, allowed := allowedSet[CourseLecturer]; allowed && tokenUser.IsLecturer {
				c.Next()
				return
			}

			if _, allowed := allowedSet[CourseEditor]; allowed && tokenUser.IsEditor {
				c.Next()
				return
			}

			if matchesCustomRole(tokenUser.CustomRolePrefix, allowedRoles, tokenUser.Roles) {
				c.Next()
				return
			}
		}

		// 3.) Check for Student.
		if _, allowed := allowedSet[CourseStudent]; allowed {
			isStudentOfCoursePhaseMiddleware()(c)
			if c.IsAborted() {
				return
			}

			tokenUser, ok = GetTokenUser(c)
			if !ok {
				log.Error("Error refreshing the token student")
				c.AbortWithStatusJSON(http.StatusUnauthorized, ErrUserNotInContext)
				return
			}

			if tokenUser.IsStudentOfCourse {
				c.Next()
				return
			}
		}

		// Access denied.
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Access denied"})
	}
}

// buildAllowedRolesSet creates a lookup set from a slice of roles.
func buildAllowedRolesSet(roles []string) map[string]struct{} {
	set := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		set[role] = struct{}{}
	}
	return set
}

// checkDirectRole returns true if a specific role is both allowed and present in the user roles.
func checkDirectRole(role string, allowedSet map[string]struct{}, userRoles map[string]bool) bool {
	if _, allowed := allowedSet[role]; allowed && userRoles[role] {
		return true
	}
	return false
}

// requiresLecturerOrCustom determines if additional checks for lecturer, editor,
// or custom roles are needed based on the allowed roles.
func requiresLecturerOrCustom(allowedSet map[string]struct{}, roles []string) bool {
	_, hasLecturer := allowedSet[CourseLecturer]
	_, hasEditor := allowedSet[CourseEditor]
	return hasLecturer || hasEditor || containsCustomRoleName(roles...)
}

func containsCustomRoleName(allowedRoles ...string) bool {
	return slices.ContainsFunc(allowedRoles, isCustomRole)
}

// builtInRoles are the roles the middleware resolves itself. They must never be
// matched against the course role prefix, which is reserved for custom group roles.
var builtInRoles = []string{PromptAdmin, PromptLecturer, CourseLecturer, CourseEditor, CourseStudent}

func isCustomRole(role string) bool {
	return !slices.Contains(builtInRoles, role)
}

// matchesCustomRole reports whether the user holds one of the course-specific
// group roles the route declares. Built-in roles are skipped because core
// resolves those itself, and an empty prefix would degrade the lookup to an
// un-prefixed role match.
func matchesCustomRole(prefix string, allowedRoles []string, userRoles map[string]bool) bool {
	if prefix == "" {
		return false
	}
	for _, role := range allowedRoles {
		if isCustomRole(role) && userRoles[prefix+role] {
			return true
		}
	}
	return false
}

// onlyContainsAdminAndLecturer returns true if the allowedSet only contains
// "PROMPT_Admin" and/or "PROMPT_Lecturer".
func onlyContainsAdminAndLecturer(allowedSet map[string]struct{}) bool {
	for role := range allowedSet {
		if role != PromptAdmin && role != PromptLecturer {
			return false
		}
	}
	return true
}
