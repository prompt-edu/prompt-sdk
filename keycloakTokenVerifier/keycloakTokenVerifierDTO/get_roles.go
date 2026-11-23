package keycloakTokenVerifierDTO

type GetCourseRoles struct {
	CourseLecturerRole string `json:"courseLecturerRole"`
	CourseEditorRole   string `json:"courseEditorRole"`
	CustomRolePrefix   string `json:"customRolePrefix"`
	// OrgAdminRoles are the Admin roles of the course's org, whose holders are Lecturer-equivalent
	// on the course. Core omits them for an ungrouped course; older cores never send them.
	OrgAdminRoles []string `json:"orgAdminRoles,omitempty"`
}
