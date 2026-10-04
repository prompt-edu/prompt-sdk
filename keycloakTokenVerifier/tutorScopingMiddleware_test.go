package keycloakTokenVerifier

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type fakeResolver struct {
	teamID   uuid.UUID
	err      error
	called   bool
	gotLogin string
}

func (f *fakeResolver) ResolveTutorTeam(_ context.Context, _ uuid.UUID, login string) (uuid.UUID, error) {
	f.called = true
	f.gotLogin = login
	return f.teamID, f.err
}

func runScoping(t *testing.T, user *TokenUser, phaseParam string, resolver *fakeResolver) (int, uuid.UUID, bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	var gotTeam uuid.UUID
	var scoped bool
	router.GET("/course_phase/:coursePhaseID", func(c *gin.Context) {
		if user != nil {
			SetTokenUser(c, *user)
		}
	}, TutorScopingMiddleware(resolver), func(c *gin.Context) {
		gotTeam, scoped = GetTutorTeamID(c)
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/course_phase/"+phaseParam, nil)
	router.ServeHTTP(w, req)
	return w.Code, gotTeam, scoped
}

func TestTutorScopingMiddleware(t *testing.T) {
	phase := uuid.New().String()
	team := uuid.New()

	t.Run("non-editor bypasses", func(t *testing.T) {
		r := &fakeResolver{teamID: team}
		code, _, scoped := runScoping(t, &TokenUser{IsEditor: false, UniversityLogin: "ab12cde"}, phase, r)
		if code != http.StatusOK || scoped || r.called {
			t.Fatalf("expected bypass without resolver call, got code=%d scoped=%v called=%v", code, scoped, r.called)
		}
	})

	t.Run("lecturer editor bypasses", func(t *testing.T) {
		r := &fakeResolver{teamID: team}
		_, _, scoped := runScoping(t, &TokenUser{IsEditor: true, IsLecturer: true, UniversityLogin: "ab12cde"}, phase, r)
		if scoped || r.called {
			t.Fatalf("lecturer must not be scoped")
		}
	})

	t.Run("phase without tutors (ErrNoRows) grants full access", func(t *testing.T) {
		r := &fakeResolver{err: pgx.ErrNoRows}
		code, _, scoped := runScoping(t, &TokenUser{IsEditor: true, UniversityLogin: "ab12cde"}, phase, r)
		if code != http.StatusOK || scoped || !r.called {
			t.Fatalf("ErrNoRows must bypass with full access, got code=%d scoped=%v", code, scoped)
		}
	})

	t.Run("editor who is not a tutor of a phase with tutors is denied", func(t *testing.T) {
		r := &fakeResolver{err: fmt.Errorf("lookup: %w", ErrNotATutor)}
		code, _, scoped := runScoping(t, &TokenUser{IsEditor: true, UniversityLogin: "ab12cde"}, phase, r)
		if code != http.StatusForbidden || scoped {
			t.Fatalf("ErrNotATutor must abort with 403, got code=%d scoped=%v", code, scoped)
		}
	})

	// A token without a login must not skip the check that denies non-tutors.
	t.Run("empty login still reaches the resolver", func(t *testing.T) {
		r := &fakeResolver{err: ErrNotATutor}
		code, _, scoped := runScoping(t, &TokenUser{IsEditor: true, UniversityLogin: "  "}, phase, r)
		if code != http.StatusForbidden || scoped || !r.called || r.gotLogin != "" {
			t.Fatalf("expected the resolver to deny an empty login, got code=%d called=%v login=%q", code, r.called, r.gotLogin)
		}
	})

	t.Run("resolver error fails closed", func(t *testing.T) {
		r := &fakeResolver{err: errors.New("db down")}
		code, _, scoped := runScoping(t, &TokenUser{IsEditor: true, UniversityLogin: "ab12cde"}, phase, r)
		if code != http.StatusInternalServerError || scoped {
			t.Fatalf("resolver error must fail closed with 500, got code=%d scoped=%v", code, scoped)
		}
	})

	t.Run("resolved tutor is scoped with normalized login", func(t *testing.T) {
		r := &fakeResolver{teamID: team}
		code, got, scoped := runScoping(t, &TokenUser{IsEditor: true, UniversityLogin: "  AB12CDE  "}, phase, r)
		if code != http.StatusOK || !scoped || got != team {
			t.Fatalf("expected scoped team %v, got code=%d team=%v scoped=%v", team, code, got, scoped)
		}
		if r.gotLogin != "ab12cde" {
			t.Fatalf("expected normalized login ab12cde, got %q", r.gotLogin)
		}
	})

	t.Run("invalid course phase id returns 400", func(t *testing.T) {
		r := &fakeResolver{teamID: team}
		code, _, _ := runScoping(t, &TokenUser{IsEditor: true, UniversityLogin: "ab12cde"}, "not-a-uuid", r)
		if code != http.StatusBadRequest || r.called {
			t.Fatalf("invalid phase id must abort 400 before resolver, got code=%d called=%v", code, r.called)
		}
	})

	t.Run("nil course phase id returns 400", func(t *testing.T) {
		r := &fakeResolver{teamID: team}
		code, _, _ := runScoping(t, &TokenUser{IsEditor: true, UniversityLogin: "ab12cde"}, uuid.Nil.String(), r)
		if code != http.StatusBadRequest || r.called {
			t.Fatalf("nil phase id must abort 400 before resolver, got code=%d called=%v", code, r.called)
		}
	})
}

func TestTutorScopingMiddleware_NilResolverPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic on nil resolver")
		}
	}()
	TutorScopingMiddleware(nil)
}
