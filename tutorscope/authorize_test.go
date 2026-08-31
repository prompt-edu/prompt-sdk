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
// two of them together rather than a hand-built context.
func authorizeOn(t *testing.T, user *keycloakTokenVerifier.TokenUser, resolver Resolver) (Access, error) {
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
	// means "may create courses", not "lecturer of this course".
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
