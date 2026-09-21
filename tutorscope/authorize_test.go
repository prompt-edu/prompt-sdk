package tutorscope

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/prompt-edu/prompt-sdk/keycloakTokenVerifier"
)

type stubResolver struct {
	teamID uuid.UUID
	err    error
}

func (s stubResolver) ResolveTutorTeam(context.Context, uuid.UUID, string) (uuid.UUID, error) {
	return s.teamID, s.err
}

// authorizeOn runs AuthorizeWrite behind the real middleware, so the test covers the
// two of them together rather than a hand-built context. It mimics a route that
// resolves course roles, which is every route admitting editors.
func authorizeOn(t *testing.T, user *keycloakTokenVerifier.TokenUser, resolver Resolver) (Access, error) {
	t.Helper()
	return authorize(t, user, resolver, true)
}

// authorizeWithoutCourseRoles mimics a route that lists PromptLecturer among its
// allowed roles: the caller is admitted on that global role alone, before their roles
// in this course are looked up, so IsLecturer and IsEditor stay false either way.
func authorizeWithoutCourseRoles(t *testing.T, user *keycloakTokenVerifier.TokenUser, resolver Resolver) (Access, error) {
	t.Helper()
	return authorize(t, user, resolver, false)
}

func authorize(t *testing.T, user *keycloakTokenVerifier.TokenUser, resolver Resolver, courseRolesResolved bool) (Access, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var (
		access Access
		gotErr error
	)
	router := gin.New()
	handlers := []gin.HandlerFunc{func(c *gin.Context) {
		if user != nil {
			keycloakTokenVerifier.SetTokenUser(c, *user)
		}
		if courseRolesResolved {
			c.Set(keycloakTokenVerifier.CourseRolesResolvedKey, true)
		}
	}}
	if resolver != nil {
		handlers = append(handlers, Middleware(resolver))
	}
	handlers = append(handlers, func(c *gin.Context) {
		access, gotErr = AuthorizeWrite(c)
		c.Status(http.StatusOK)
	})
	router.PUT("/course_phase/:coursePhaseID", handlers...)

	req := httptest.NewRequest(http.MethodPut, "/course_phase/"+uuid.New().String(), nil)
	router.ServeHTTP(httptest.NewRecorder(), req)
	return access, gotErr
}

func TestAuthorizeWrite(t *testing.T) {
	team := uuid.New()
	other := uuid.New()

	t.Run("no token user is not authenticated", func(t *testing.T) {
		_, err := authorizeOn(t, nil, stubResolver{teamID: team})
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Fatalf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("admin writes any team", func(t *testing.T) {
		user := &keycloakTokenVerifier.TokenUser{
			Roles: map[string]bool{keycloakTokenVerifier.PromptAdmin: true},
		}
		access, err := authorizeOn(t, user, stubResolver{teamID: team})
		if err != nil || access.Confined || !access.Unrestricted() {
			t.Fatalf("admin must be unrestricted, got access=%+v err=%v", access, err)
		}
		if !access.AllowsTeam(other) {
			t.Fatalf("an unrestricted caller must be allowed any team")
		}
		if access.Guard().Valid {
			t.Fatalf("an unrestricted caller must produce a NULL guard")
		}
	})

	t.Run("course lecturer writes any team", func(t *testing.T) {
		user := &keycloakTokenVerifier.TokenUser{IsLecturer: true}
		access, err := authorizeOn(t, user, stubResolver{teamID: team})
		if err != nil || access.Confined {
			t.Fatalf("lecturer must be unrestricted, got access=%+v err=%v", access, err)
		}
	})

	t.Run("student is denied", func(t *testing.T) {
		user := &keycloakTokenVerifier.TokenUser{IsStudentOfCourse: true}
		_, err := authorizeOn(t, user, stubResolver{teamID: team})
		if !errors.Is(err, ErrWriteDenied) {
			t.Fatalf("expected ErrWriteDenied, got %v", err)
		}
	})

	t.Run("tutor editor is confined to their team", func(t *testing.T) {
		user := &keycloakTokenVerifier.TokenUser{IsEditor: true, UniversityLogin: "ab12cde"}
		access, err := authorizeOn(t, user, stubResolver{teamID: team})
		if err != nil || !access.Confined || access.TeamID != team {
			t.Fatalf("expected confinement to %v, got access=%+v err=%v", team, access, err)
		}
		if !access.AllowsTeam(team) || access.AllowsTeam(other) {
			t.Fatalf("a confined caller must be allowed only their own team")
		}
		guard := access.Guard()
		if !guard.Valid || uuid.UUID(guard.Bytes) != team {
			t.Fatalf("expected guard %v, got %+v", team, guard)
		}
	})

	// The read middleware lets these editors through with full access on purpose.
	// Writes must not inherit that.
	t.Run("editor with no tutor row is denied", func(t *testing.T) {
		user := &keycloakTokenVerifier.TokenUser{IsEditor: true, UniversityLogin: "ab12cde"}
		_, err := authorizeOn(t, user, stubResolver{err: pgx.ErrNoRows})
		if !errors.Is(err, ErrWriteDenied) {
			t.Fatalf("an unresolvable editor must be denied, got %v", err)
		}
	})

	t.Run("editor with no university login is denied", func(t *testing.T) {
		user := &keycloakTokenVerifier.TokenUser{IsEditor: true}
		_, err := authorizeOn(t, user, stubResolver{teamID: team})
		if !errors.Is(err, ErrWriteDenied) {
			t.Fatalf("an editor without a login must be denied, got %v", err)
		}
	})

	// A tutor who also holds the global PROMPT_Lecturer role stays scoped: that role
	// means "may create courses", not "lecturer of this course". This is the route
	// that resolved course roles, so the editor role is known.
	t.Run("prompt lecturer who is a tutor stays confined", func(t *testing.T) {
		user := &keycloakTokenVerifier.TokenUser{
			Roles:           map[string]bool{keycloakTokenVerifier.PromptLecturer: true},
			IsEditor:        true,
			UniversityLogin: "ab12cde",
		}
		access, err := authorizeOn(t, user, stubResolver{teamID: team})
		if err != nil || !access.Confined || access.TeamID != team {
			t.Fatalf("expected confinement to %v, got access=%+v err=%v", team, access, err)
		}
	})

	t.Run("missing middleware is a wiring bug, not a denial", func(t *testing.T) {
		user := &keycloakTokenVerifier.TokenUser{IsEditor: true, UniversityLogin: "ab12cde"}
		_, err := authorizeOn(t, user, nil)
		if !errors.Is(err, ErrScopingNotApplied) {
			t.Fatalf("expected ErrScopingNotApplied, got %v", err)
		}
	})

	// Without the middleware a lecturer or admin still resolves, so a read-only route
	// that never installed it can still authorize privileged writes.
	t.Run("missing middleware still resolves privileged callers", func(t *testing.T) {
		user := &keycloakTokenVerifier.TokenUser{IsLecturer: true}
		access, err := authorizeOn(t, user, nil)
		if err != nil || access.Confined {
			t.Fatalf("lecturer must resolve without the middleware, got access=%+v err=%v", access, err)
		}
	})

	// On a route listing PromptLecturer the course's own lecturer arrives with no
	// course role set, exactly like a tutor would. Denying them would lock every
	// lecturer out of every write on that route.
	t.Run("unresolved course roles are a wiring bug, not a denial", func(t *testing.T) {
		user := &keycloakTokenVerifier.TokenUser{
			Roles: map[string]bool{keycloakTokenVerifier.PromptLecturer: true},
		}
		_, err := authorizeWithoutCourseRoles(t, user, stubResolver{teamID: team})
		if !errors.Is(err, ErrCourseRolesNotResolved) {
			t.Fatalf("expected ErrCourseRolesNotResolved, got %v", err)
		}
	})

	t.Run("an unresolved caller without the global role is still denied", func(t *testing.T) {
		user := &keycloakTokenVerifier.TokenUser{IsStudentOfCourse: true}
		_, err := authorizeWithoutCourseRoles(t, user, stubResolver{teamID: team})
		if !errors.Is(err, ErrWriteDenied) {
			t.Fatalf("expected ErrWriteDenied, got %v", err)
		}
	})

	// A caller that ignores the error must still be unable to write.
	t.Run("a denied caller carries no write scope", func(t *testing.T) {
		user := &keycloakTokenVerifier.TokenUser{IsStudentOfCourse: true}
		access, err := authorizeOn(t, user, stubResolver{teamID: team})
		if err == nil {
			t.Fatalf("expected a denial")
		}
		assertGrantsNothing(t, access, team)
	})
}

// Access is returned by value, so a service can hold a zero one. It must grant
// nothing, or an ignored error turns into an unrestricted write.
func TestZeroAccessGrantsNothing(t *testing.T) {
	assertGrantsNothing(t, Access{}, uuid.New())
}

func assertGrantsNothing(t *testing.T, access Access, team uuid.UUID) {
	t.Helper()
	if access.Unrestricted() {
		t.Fatalf("expected no access, got unrestricted")
	}
	if access.AllowsTeam(team) || access.AllowsTeam(uuid.Nil) {
		t.Fatalf("expected no team to be allowed, got access=%+v", access)
	}
	guard := access.Guard()
	if !guard.Valid || uuid.UUID(guard.Bytes) != uuid.Nil {
		t.Fatalf("expected a guard no row matches, got %+v", guard)
	}
}

func TestNormalizeLogin(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"  AB12CDE  ", "ab12cde"},
		{"ab12cde", "ab12cde"},
		{"   ", ""},
		{"", ""},
	} {
		if got := NormalizeLogin(tc.in); got != tc.want {
			t.Fatalf("NormalizeLogin(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
