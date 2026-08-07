package audit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSink captures events synchronously for assertions.
type fakeSink struct {
	mu     sync.Mutex
	events []Event
}

func (f *fakeSink) Record(_ context.Context, e Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, e)
	return nil
}

func (f *fakeSink) all() []Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Event, len(f.events))
	copy(out, f.events)
	return out
}

func staticActor(c *gin.Context) (Actor, bool) {
	if c.GetBool("noActor") {
		return Actor{}, false
	}
	return Actor{ID: "u1", Name: "Ada Lovelace", Email: "ada@tum.de", Role: "Lecturer", Roles: []string{"Lecturer"}}, true
}

// run executes a single request through a router with the audit middleware and
// returns the events the sink saw. The middleware writes in a background
// goroutine, so we wait on a WaitGroup toggled by a wrapper sink.
func run(t *testing.T, sink Sink, method, path string, register func(r *gin.Engine)) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware(sink, WithActorExtractor(staticActor), WithSourceService("core")))
	register(r)

	req := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
}

// waitSink blocks in Record until released, letting tests deterministically
// observe the async write.
type waitSink struct {
	*fakeSink
	wg *sync.WaitGroup
}

func (s waitSink) Record(ctx context.Context, e Event) error {
	defer s.wg.Done()
	return s.fakeSink.Record(ctx, e)
}

func newWaitSink(n int) (waitSink, *sync.WaitGroup) {
	wg := &sync.WaitGroup{}
	wg.Add(n)
	return waitSink{fakeSink: &fakeSink{}, wg: wg}, wg
}

func TestMiddleware_LogsSuccessfulMutation(t *testing.T) {
	sink, wg := newWaitSink(1)
	run(t, sink, http.MethodPost, "/api/course_phase/p1/slots", func(r *gin.Engine) {
		r.POST("/api/course_phase/:coursePhaseID/slots", func(c *gin.Context) { c.Status(http.StatusCreated) })
	})
	wg.Wait()

	events := sink.all()
	require.Len(t, events, 1)
	e := events[0]
	assert.Equal(t, OutcomeSuccess, e.Outcome)
	assert.Equal(t, "Created slot", e.Action)
	assert.Equal(t, "POST /api/course_phase/:coursePhaseID/slots", e.ActionKey)
	assert.Equal(t, "p1", e.CoursePhaseID)
	assert.Equal(t, "u1", e.ActorID)
	assert.Equal(t, "Lecturer", e.ActorRole)
	assert.Equal(t, "core", e.SourceService)
}

func TestMiddleware_LogsDeniedAs403(t *testing.T) {
	sink, wg := newWaitSink(1)
	run(t, sink, http.MethodDelete, "/api/teams/t1", func(r *gin.Engine) {
		r.DELETE("/api/teams/:uuid", func(c *gin.Context) { c.Status(http.StatusForbidden) })
	})
	wg.Wait()

	events := sink.all()
	require.Len(t, events, 1)
	assert.Equal(t, OutcomeDenied, events[0].Outcome)
	assert.Equal(t, "t1", events[0].EntityID)
	assert.Equal(t, "Deleted team", events[0].Action)
}

func TestMiddleware_SkipsReadsAndNoise(t *testing.T) {
	for _, tc := range []struct {
		name, method string
		status       int
		noActor      bool
	}{
		{"GET read", http.MethodGet, http.StatusOK, false},
		{"401 unauth", http.MethodPost, http.StatusUnauthorized, false},
		{"404 not found", http.MethodPost, http.StatusNotFound, false},
		{"422 validation", http.MethodPost, http.StatusUnprocessableEntity, false},
		{"no actor", http.MethodPost, http.StatusCreated, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sink := &fakeSink{}
			run(t, sink, tc.method, "/api/things", func(r *gin.Engine) {
				r.Handle(tc.method, "/api/things", func(c *gin.Context) {
					c.Set("noActor", tc.noActor)
					c.Status(tc.status)
				})
			})
			assert.Empty(t, sink.all())
		})
	}
}

func TestMiddleware_DescribeOverridesLabel(t *testing.T) {
	sink, wg := newWaitSink(1)
	run(t, sink, http.MethodPost, "/api/courses/c1/copy", func(r *gin.Engine) {
		r.POST("/api/courses/:courseId/copy", Describe("Copied course"), func(c *gin.Context) { c.Status(http.StatusOK) })
	})
	wg.Wait()

	events := sink.all()
	require.Len(t, events, 1)
	assert.Equal(t, "Copied course", events[0].Action)
	assert.Equal(t, "c1", events[0].CourseID)
}

func TestMiddleware_SkipPreventsEntry(t *testing.T) {
	sink := &fakeSink{}
	run(t, sink, http.MethodPost, "/api/sync/pull", func(r *gin.Engine) {
		r.POST("/api/sync/pull", Skip(), func(c *gin.Context) { c.Status(http.StatusOK) })
	})
	assert.Empty(t, sink.all())
}

func TestMiddleware_SuppressPreventsEntry(t *testing.T) {
	sink := &fakeSink{}
	run(t, sink, http.MethodPost, "/api/noop", func(r *gin.Engine) {
		r.POST("/api/noop", func(c *gin.Context) {
			Suppress(c)
			c.Status(http.StatusOK)
		})
	})
	assert.Empty(t, sink.all())
}

func TestRecord_AutoSuppressesBackstop(t *testing.T) {
	sink, wg := newWaitSink(1)
	run(t, sink, http.MethodPost, "/api/grades", func(r *gin.Engine) {
		r.POST("/api/grades", func(c *gin.Context) {
			Record(c, Event{Action: "Published grades", EntityType: "grade", EntityID: "g1", EntityName: "team Alpha"})
			c.Status(http.StatusOK)
		})
	})
	wg.Wait()

	events := sink.all()
	require.Len(t, events, 1) // exactly one: the explicit Record, not the backstop
	assert.Equal(t, "Published grades", events[0].Action)
	assert.Equal(t, "team Alpha", events[0].EntityName)
	assert.Equal(t, "u1", events[0].ActorID) // actor filled from extractor
}

func TestRecord_StillWritesWhenSuppressed(t *testing.T) {
	sink, wg := newWaitSink(1)
	run(t, sink, http.MethodPost, "/api/thing", func(r *gin.Engine) {
		r.POST("/api/thing", func(c *gin.Context) {
			Suppress(c) // silences the backstop
			Record(c, Event{Action: "Did the thing on purpose"})
			c.Status(http.StatusOK)
		})
	})
	wg.Wait()

	events := sink.all()
	require.Len(t, events, 1)
	assert.Equal(t, "Did the thing on purpose", events[0].Action)
}

func TestMiddleware_CapturesDenialFromAbortingMiddleware(t *testing.T) {
	sink, wg := newWaitSink(1)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware(sink, WithActorExtractor(staticActor), WithSourceService("core")))
	// A downstream authorization middleware that aborts before the handler runs.
	authz := func(c *gin.Context) { c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "denied"}) }
	r.POST("/api/course_phase/:coursePhaseID/grades", authz, func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/api/course_phase/p1/grades", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)
	wg.Wait()

	events := sink.all()
	require.Len(t, events, 1)
	assert.Equal(t, OutcomeDenied, events[0].Outcome)
	assert.Equal(t, "u1", events[0].ActorID)
}

func TestRecord_SkipsWhenNoActor(t *testing.T) {
	sink := &fakeSink{}
	run(t, sink, http.MethodPost, "/api/thing", func(r *gin.Engine) {
		r.POST("/api/thing", func(c *gin.Context) {
			c.Set("noActor", true)
			Record(c, Event{Action: "Orphan event"}) // no actor -> must not write a blank-actor entry
			c.Status(http.StatusOK)
		})
	})
	assert.Empty(t, sink.all())
}

func TestMiddleware_NilSinkIsNoOp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Middleware(nil))
	called := false
	r.POST("/api/thing", func(c *gin.Context) { called = true; c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/api/thing", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)
	assert.True(t, called)
}

func TestEnabled(t *testing.T) {
	t.Setenv("AUDIT_ENABLED", "true")
	assert.True(t, Enabled())
	t.Setenv("AUDIT_ENABLED", "")
	assert.False(t, Enabled())
}

func TestNewCoreSink_DisabledReturnsNil(t *testing.T) {
	t.Setenv("AUDIT_ENABLED", "true")
	t.Setenv("AUDIT_INGEST_KEY", "")
	assert.Nil(t, NewCoreSink("http://core", "interview"))

	t.Setenv("AUDIT_ENABLED", "")
	t.Setenv("AUDIT_INGEST_KEY", "secret")
	assert.Nil(t, NewCoreSink("http://core", "interview"))
}

func TestNewCoreSink_SendsHeaders(t *testing.T) {
	t.Setenv("AUDIT_ENABLED", "true")
	t.Setenv("AUDIT_INGEST_KEY", "secret")

	var gotService, gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotService = r.Header.Get("X-Audit-Service")
		gotToken = r.Header.Get("X-Audit-Token")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	sink := NewCoreSink(srv.URL, "interview")
	require.NotNil(t, sink)
	require.NoError(t, sink.Record(context.Background(), Event{Action: "Created slot"}))
	assert.Equal(t, "interview", gotService)
	assert.Equal(t, "secret", gotToken)
}

func TestDeriveAction(t *testing.T) {
	assert.Equal(t, "Created slot", deriveAction("POST", "/api/course_phase/:id/slots"))
	assert.Equal(t, "Updated participation", deriveAction("PATCH", "/api/.../participations"))
	assert.Equal(t, "Deleted team", deriveAction("DELETE", "/api/teams/:uuid"))
}
