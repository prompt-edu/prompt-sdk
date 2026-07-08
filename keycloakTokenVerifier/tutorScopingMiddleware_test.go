package keycloakTokenVerifier

import (
	"context"
	"errors"
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

	t.Run("empty login bypasses", func(t *testing.T) {
		r := &fakeResolver{teamID: team}
		_, _, scoped := runScoping(t, &TokenUser{IsEditor: true, UniversityLogin: "  "}, phase, r)
		if scoped || r.called {
			t.Fatalf("empty login must not call resolver")
		}
	})

	t.Run("not a tutor (ErrNoRows) grants full access", func(t *testing.T) {
		r := &fakeResolver{err: pgx.ErrNoRows}
		code, _, scoped := runScoping(t, &TokenUser{IsEditor: true, UniversityLogin: "ab12cde"}, phase, r)
		if code != http.StatusOK || scoped || !r.called {
			t.Fatalf("ErrNoRows must bypass with full access, got code=%d scoped=%v", code, scoped)
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
}
