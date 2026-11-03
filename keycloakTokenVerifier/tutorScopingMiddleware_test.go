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

type fakeTeamsResolver struct {
	teamIDs  []uuid.UUID
	err      error
	called   bool
	gotLogin string
}

func (f *fakeTeamsResolver) ResolveTutorTeams(_ context.Context, _ uuid.UUID, login string) ([]uuid.UUID, error) {
	f.called = true
	f.gotLogin = login
	return f.teamIDs, f.err
}

type teamsScopingResult struct {
	code     int
	teamIDs  []uuid.UUID
	scoped   bool
	firstID  uuid.UUID
	firstSet bool
}

func runTeamsScoping(t *testing.T, user *TokenUser, resolver TutorTeamsResolver) teamsScopingResult {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var result teamsScopingResult
	router := gin.New()
	router.GET("/course_phase/:coursePhaseID", func(c *gin.Context) {
		if user != nil {
			SetTokenUser(c, *user)
		}
	}, TutorTeamsScopingMiddleware(resolver), func(c *gin.Context) {
		result.teamIDs, result.scoped = GetTutorTeamIDs(c)
		result.firstID, result.firstSet = GetTutorTeamID(c)
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/course_phase/"+uuid.New().String(), nil)
	router.ServeHTTP(w, req)
	result.code = w.Code
	return result
}

func TestTutorTeamsScopingMiddleware(t *testing.T) {
	teamA := uuid.New()
	teamB := uuid.New()
	editor := &TokenUser{IsEditor: true, UniversityLogin: "  AB12CDE "}

	t.Run("tutor of one team is scoped to it", func(t *testing.T) {
		r := &fakeTeamsResolver{teamIDs: []uuid.UUID{teamA}}
		got := runTeamsScoping(t, editor, r)
		if got.code != http.StatusOK || !got.scoped || len(got.teamIDs) != 1 || got.teamIDs[0] != teamA {
			t.Fatalf("expected scope [%v], got %+v", teamA, got)
		}
		if r.gotLogin != "ab12cde" {
			t.Fatalf("expected normalized login ab12cde, got %q", r.gotLogin)
		}
	})

	t.Run("tutor of several teams is scoped to all of them", func(t *testing.T) {
		got := runTeamsScoping(t, editor, &fakeTeamsResolver{teamIDs: []uuid.UUID{teamA, teamB}})
		if got.code != http.StatusOK || !got.scoped || len(got.teamIDs) != 2 || got.teamIDs[0] != teamA || got.teamIDs[1] != teamB {
			t.Fatalf("expected scope [%v %v], got %+v", teamA, teamB, got)
		}
	})

	// A caller that only knows one team must see less, never more.
	t.Run("deprecated single-team getter returns the first team", func(t *testing.T) {
		got := runTeamsScoping(t, editor, &fakeTeamsResolver{teamIDs: []uuid.UUID{teamA, teamB}})
		if !got.firstSet || got.firstID != teamA {
			t.Fatalf("expected first team %v, got %v (set=%v)", teamA, got.firstID, got.firstSet)
		}
	})

	t.Run("empty team list fails closed", func(t *testing.T) {
		got := runTeamsScoping(t, editor, &fakeTeamsResolver{teamIDs: []uuid.UUID{}})
		if got.code != http.StatusForbidden || got.scoped || got.firstSet {
			t.Fatalf("an empty list must abort with 403, got %+v", got)
		}
	})

	t.Run("phase without tutors (ErrNoRows) grants full access", func(t *testing.T) {
		got := runTeamsScoping(t, editor, &fakeTeamsResolver{err: pgx.ErrNoRows})
		if got.code != http.StatusOK || got.scoped || got.firstSet {
			t.Fatalf("ErrNoRows must bypass unscoped, got %+v", got)
		}
	})

	t.Run("editor who is not a tutor is denied", func(t *testing.T) {
		got := runTeamsScoping(t, editor, &fakeTeamsResolver{err: fmt.Errorf("lookup: %w", ErrNotATutor)})
		if got.code != http.StatusForbidden || got.scoped {
			t.Fatalf("ErrNotATutor must abort with 403, got %+v", got)
		}
	})

	t.Run("resolver error fails closed", func(t *testing.T) {
		got := runTeamsScoping(t, editor, &fakeTeamsResolver{err: errors.New("db down")})
		if got.code != http.StatusInternalServerError || got.scoped {
			t.Fatalf("resolver error must fail closed with 500, got %+v", got)
		}
	})

	t.Run("lecturer bypasses without a lookup", func(t *testing.T) {
		r := &fakeTeamsResolver{teamIDs: []uuid.UUID{teamA}}
		got := runTeamsScoping(t, &TokenUser{IsEditor: true, IsLecturer: true, UniversityLogin: "ab12cde"}, r)
		if got.code != http.StatusOK || got.scoped || r.called {
			t.Fatalf("lecturer must not be scoped, got %+v called=%v", got, r.called)
		}
	})

	// The returned slice is a copy, so a handler cannot rewrite the scope of a later one.
	t.Run("handlers get a copy of the scope", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		router := gin.New()
		var second []uuid.UUID
		router.GET("/course_phase/:coursePhaseID", func(c *gin.Context) {
			SetTokenUser(c, *editor)
		}, TutorTeamsScopingMiddleware(&fakeTeamsResolver{teamIDs: []uuid.UUID{teamA, teamB}}), func(c *gin.Context) {
			first, _ := GetTutorTeamIDs(c)
			first[0] = uuid.Nil
			second, _ = GetTutorTeamIDs(c)
			c.Status(http.StatusOK)
		})
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/course_phase/"+uuid.New().String(), nil))
		if len(second) != 2 || second[0] != teamA {
			t.Fatalf("expected an unchanged scope, got %v", second)
		}
	})
}

// The single-team middleware must keep its contract on top of the multi-team one.
func TestTutorScopingMiddleware_ExposesTheTeamAsAList(t *testing.T) {
	team := uuid.New()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	var (
		teamIDs []uuid.UUID
		scoped  bool
	)
	router.GET("/course_phase/:coursePhaseID", func(c *gin.Context) {
		SetTokenUser(c, TokenUser{IsEditor: true, UniversityLogin: "ab12cde"})
	}, TutorScopingMiddleware(&fakeResolver{teamID: team}), func(c *gin.Context) {
		teamIDs, scoped = GetTutorTeamIDs(c)
		c.Status(http.StatusOK)
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/course_phase/"+uuid.New().String(), nil))
	if !scoped || len(teamIDs) != 1 || teamIDs[0] != team {
		t.Fatalf("expected [%v], got %v (scoped=%v)", team, teamIDs, scoped)
	}
}

func TestTutorTeamsScopingMiddleware_NilResolverPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic on nil resolver")
		}
	}()
	TutorTeamsScopingMiddleware(nil)
}
